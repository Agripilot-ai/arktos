/*
Copyright 2018 The Kubernetes Authors.
Copyright 2020 Authors of Arktos - file modified.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package manager

import (
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/sets"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	podutil "k8s.io/kubernetes/pkg/api/v1/pod"
	"k8s.io/kubernetes/pkg/kubelet/kubeclientmanager"

	"github.com/stretchr/testify/assert"
)

func checkObject(t *testing.T, store *objectStore, tenant, ns, name string, shouldExist bool) {
	_, err := store.Get(tenant, ns, name, 0)
	if shouldExist && err != nil {
		t.Errorf("unexpected actions: %#v", err)
	}
	if !shouldExist && (err == nil || !strings.Contains(err.Error(), fmt.Sprintf("object %q/%q/%q not registered", tenant, ns, name))) {
		t.Errorf("unexpected actions: %#v", err)
	}
}

func noObjectTTL() (time.Duration, bool) {
	return time.Duration(0), false
}

func getSecret(fakeClient clientset.Interface) GetObjectFunc {
	return func(tenant, namespace, name string, originClientID int, opts metav1.GetOptions) (runtime.Object, error) {
		return fakeClient.CoreV1().SecretsWithMultiTenancy(namespace, tenant).Get(name, opts)
	}
}

func newSecretStore(fakeClient clientset.Interface, clock clock.Clock, getTTL GetObjectTTLFunc, ttl time.Duration) *objectStore {
	return &objectStore{
		getObject:  getSecret(fakeClient),
		clock:      clock,
		items:      make(map[objectKey]*objectStoreItem),
		defaultTTL: ttl,
		getTTL:     getTTL,
	}
}

func getSecretNames(pod *v1.Pod) sets.String {
	result := sets.NewString()
	podutil.VisitPodSecretNames(pod, func(name string) bool {
		result.Insert(name)
		return true
	})
	return result
}

func newCacheBasedSecretManager(store Store) Manager {
	return NewCacheBasedManager(store, getSecretNames)
}

func TestSecretStore(t *testing.T) {
	testSecretStore(t, metav1.TenantSystem)
}

func TestSecretStoreWithMultiTenancy(t *testing.T) {
	testSecretStore(t, "test-te")
}

func testSecretStore(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	store := newSecretStore(fakeClient, clock.RealClock{}, noObjectTTL, 0)
	store.AddReference(tenant, "ns1", "name1", 0)
	store.AddReference(tenant, "ns2", "name2", 0)
	store.AddReference(tenant, "ns1", "name1", 0)
	store.AddReference(tenant, "ns1", "name1", 0)
	store.DeleteReference(tenant, "ns1", "name1", 0)
	store.DeleteReference(tenant, "ns2", "name2", 0)
	store.AddReference(tenant, "ns3", "name3", 0)

	// Adds don't issue Get requests.
	actions := fakeClient.Actions()
	assert.Equal(t, 0, len(actions), "unexpected actions: %#v", actions)
	// Should issue Get request
	store.Get(tenant, "ns1", "name1", 0)
	// Shouldn't issue Get request, as secret is not registered
	store.Get(tenant, "ns2", "name2", 0)
	// Should issue Get request
	store.Get(tenant, "ns3", "name3", 0)

	actions = fakeClient.Actions()
	assert.Equal(t, 2, len(actions), "unexpected actions: %#v", actions)

	for _, a := range actions {
		assert.True(t, a.Matches("get", "secrets"), "unexpected actions: %#v", a)
	}

	checkObject(t, store, tenant, "ns1", "name1", true)
	checkObject(t, store, tenant, "ns2", "name2", false)
	checkObject(t, store, tenant, "ns3", "name3", true)
	checkObject(t, store, tenant, "ns4", "name4", false)
}

func TestSecretStoreDeletingSecret(t *testing.T) {
	testSecretStoreDeletingSecret(t, metav1.TenantSystem)
}

func TestSecretStoreDeletingSecretWithMultiTenancy(t *testing.T) {
	testSecretStoreDeletingSecret(t, "test-te")
}

func testSecretStoreDeletingSecret(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	store := newSecretStore(fakeClient, clock.RealClock{}, noObjectTTL, 0)
	store.AddReference(tenant, "ns", "name", 0)

	result := &v1.Secret{ObjectMeta: metav1.ObjectMeta{Tenant: tenant, Namespace: "ns", Name: "name", ResourceVersion: "10"}}
	fakeClient.AddReactor("get", "secrets", func(action core.Action) (bool, runtime.Object, error) {
		return true, result, nil
	})
	secret, err := store.Get(tenant, "ns", "name", 0)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if !reflect.DeepEqual(secret, result) {
		t.Errorf("Unexpected secret: %v", secret)
	}

	fakeClient.PrependReactor("get", "secrets", func(action core.Action) (bool, runtime.Object, error) {
		return true, &v1.Secret{}, apierrors.NewNotFound(v1.Resource("secret"), "name")
	})
	secret, err = store.Get(tenant, "ns", "name", 0)
	if err == nil || !apierrors.IsNotFound(err) {
		t.Errorf("Unexpected error: %v", err)
	}
	if !reflect.DeepEqual(secret, &v1.Secret{}) {
		t.Errorf("Unexpected secret: %v", secret)
	}
}

func TestSecretStoreGetAlwaysRefresh(t *testing.T) {
	testSecretStoreGetAlwaysRefresh(t, metav1.TenantSystem)
}

func TestSecretStoreGetAlwaysRefreshWithMultiTenancy(t *testing.T) {
	testSecretStoreGetAlwaysRefresh(t, "test-te")
}

func testSecretStoreGetAlwaysRefresh(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	fakeClock := clock.NewFakeClock(time.Now())
	store := newSecretStore(fakeClient, fakeClock, noObjectTTL, 0)

	for i := 0; i < 10; i++ {
		store.AddReference(tenant, fmt.Sprintf("ns-%d", i), fmt.Sprintf("name-%d", i), 0)
	}
	fakeClient.ClearActions()

	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(i int) {
			store.Get(tenant, fmt.Sprintf("ns-%d", i%10), fmt.Sprintf("name-%d", i%10), 0)
			wg.Done()
		}(i)
	}
	wg.Wait()
	actions := fakeClient.Actions()
	assert.Equal(t, 100, len(actions), "unexpected actions: %#v", actions)

	for _, a := range actions {
		assert.True(t, a.Matches("get", "secrets"), "unexpected actions: %#v", a)
	}
}

func TestSecretStoreGetNeverRefresh(t *testing.T) {
	testSecretStoreGetNeverRefresh(t, metav1.TenantSystem)
}

func TestSecretStoreGetNeverRefreshWithMultiTenancy(t *testing.T) {
	testSecretStoreGetNeverRefresh(t, "test-te")
}

func testSecretStoreGetNeverRefresh(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	fakeClock := clock.NewFakeClock(time.Now())
	store := newSecretStore(fakeClient, fakeClock, noObjectTTL, time.Minute)

	for i := 0; i < 10; i++ {
		store.AddReference(tenant, fmt.Sprintf("ns-%d", i), fmt.Sprintf("name-%d", i), 0)
	}
	fakeClient.ClearActions()

	wg := sync.WaitGroup{}
	wg.Add(100)
	for i := 0; i < 100; i++ {
		go func(i int) {
			store.Get(tenant, fmt.Sprintf("ns-%d", i%10), fmt.Sprintf("name-%d", i%10), 0)
			wg.Done()
		}(i)
	}
	wg.Wait()
	actions := fakeClient.Actions()
	// Only first Get, should forward the Get request.
	assert.Equal(t, 10, len(actions), "unexpected actions: %#v", actions)
}

func TestCustomTTL(t *testing.T) {
	testCustomTTL(t, metav1.TenantSystem)
}

func TestCustomTTLWithMultiTenancy(t *testing.T) {
	testCustomTTL(t, "test-te")
}

func testCustomTTL(t *testing.T, tenant string) {
	ttl := time.Duration(0)
	ttlExists := false
	customTTL := func() (time.Duration, bool) {
		return ttl, ttlExists
	}

	fakeClient := &fake.Clientset{}
	fakeClock := clock.NewFakeClock(time.Time{})
	store := newSecretStore(fakeClient, fakeClock, customTTL, time.Minute)

	store.AddReference(tenant, "ns", "name", 0)
	store.Get(tenant, "ns", "name", 0)
	fakeClient.ClearActions()

	// Set 0-ttl and see if that works.
	ttl = time.Duration(0)
	ttlExists = true
	store.Get(tenant, "ns", "name", 0)
	actions := fakeClient.Actions()
	assert.Equal(t, 1, len(actions), "unexpected actions: %#v", actions)
	fakeClient.ClearActions()

	// Set 5-minute ttl and see if this works.
	ttl = time.Duration(5) * time.Minute
	store.Get(tenant, "ns", "name", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 0, len(actions), "unexpected actions: %#v", actions)
	// Still no effect after 4 minutes.
	fakeClock.Step(4 * time.Minute)
	store.Get(tenant, "ns", "name", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 0, len(actions), "unexpected actions: %#v", actions)
	// Now it should have an effect.
	fakeClock.Step(time.Minute)
	store.Get(tenant, "ns", "name", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 1, len(actions), "unexpected actions: %#v", actions)
	fakeClient.ClearActions()

	// Now remove the custom ttl and see if that works.
	ttlExists = false
	fakeClock.Step(55 * time.Second)
	store.Get(tenant, "ns", "name", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 0, len(actions), "unexpected actions: %#v", actions)
	// Pass the minute and it should be triggered now.
	fakeClock.Step(5 * time.Second)
	store.Get(tenant, "ns", "name", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 1, len(actions), "unexpected actions: %#v", actions)
}

func TestParseNodeAnnotation(t *testing.T) {
	testCases := []struct {
		node   *v1.Node
		err    error
		exists bool
		ttl    time.Duration
	}{
		{
			node:   nil,
			err:    fmt.Errorf("error"),
			exists: false,
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name: "node",
				},
			},
			exists: false,
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node",
					Annotations: map[string]string{},
				},
			},
			exists: false,
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node",
					Annotations: map[string]string{v1.ObjectTTLAnnotationKey: "bad"},
				},
			},
			exists: false,
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node",
					Annotations: map[string]string{v1.ObjectTTLAnnotationKey: "0"},
				},
			},
			exists: true,
			ttl:    time.Duration(0),
		},
		{
			node: &v1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node",
					Annotations: map[string]string{v1.ObjectTTLAnnotationKey: "60"},
				},
			},
			exists: true,
			ttl:    time.Minute,
		},
	}
	for i, testCase := range testCases {
		getNode := func() (*v1.Node, error) { return testCase.node, testCase.err }
		ttl, exists := GetObjectTTLFromNodeFunc(getNode)()
		if exists != testCase.exists {
			t.Errorf("%d: incorrect parsing: %t", i, exists)
			continue
		}
		if exists && ttl != testCase.ttl {
			t.Errorf("%d: incorrect ttl: %v", i, ttl)
		}
	}
}

type envSecrets struct {
	envVarNames  []string
	envFromNames []string
}

type secretsToAttach struct {
	imagePullSecretNames []string
	containerEnvSecrets  []envSecrets
}

func podWithSecrets(tenant, ns, podName string, toAttach secretsToAttach) *v1.Pod {
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Tenant:    tenant,
			Namespace: ns,
			Name:      podName,
		},
		Spec: v1.PodSpec{},
	}
	for _, name := range toAttach.imagePullSecretNames {
		pod.Spec.ImagePullSecrets = append(
			pod.Spec.ImagePullSecrets, v1.LocalObjectReference{Name: name})
	}
	for i, secrets := range toAttach.containerEnvSecrets {
		container := v1.Container{
			Name: fmt.Sprintf("container-%d", i),
		}
		for _, name := range secrets.envFromNames {
			envFrom := v1.EnvFromSource{
				SecretRef: &v1.SecretEnvSource{
					LocalObjectReference: v1.LocalObjectReference{
						Name: name,
					},
				},
			}
			container.EnvFrom = append(container.EnvFrom, envFrom)
		}

		for _, name := range secrets.envVarNames {
			envSource := &v1.EnvVarSource{
				SecretKeyRef: &v1.SecretKeySelector{
					LocalObjectReference: v1.LocalObjectReference{
						Name: name,
					},
				},
			}
			container.Env = append(container.Env, v1.EnvVar{ValueFrom: envSource})
		}
		pod.Spec.Containers = append(pod.Spec.Containers, container)
	}
	return pod
}

func TestCacheInvalidation(t *testing.T) {
	testCacheInvalidation(t, metav1.TenantSystem)
}

func TestCacheInvalidationWithMultiTenancy(t *testing.T) {
	testCacheInvalidation(t, "test-te")
}

func testCacheInvalidation(t *testing.T, tenant string) {
	kubeclientmanager.NewKubeClientManager()
	fakeClient := &fake.Clientset{}
	fakeClock := clock.NewFakeClock(time.Now())
	store := newSecretStore(fakeClient, fakeClock, noObjectTTL, time.Minute)
	manager := newCacheBasedSecretManager(store)

	// Create a pod with some secrets.
	s1 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s1"}, envFromNames: []string{"s10"}},
			{envVarNames: []string{"s2"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	// Fetch both secrets - this should triggger get operations.
	store.Get(tenant, "ns1", "s1", 0)
	store.Get(tenant, "ns1", "s10", 0)
	store.Get(tenant, "ns1", "s2", 0)
	actions := fakeClient.Actions()
	assert.Equal(t, 3, len(actions), "unexpected actions: %#v", actions)
	fakeClient.ClearActions()

	// Update a pod with a new secret.
	s2 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s1"}},
			{envVarNames: []string{"s2"}, envFromNames: []string{"s20"}},
			{envVarNames: []string{"s3"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s2))
	// All secrets should be invalidated - this should trigger get operations.
	store.Get(tenant, "ns1", "s1", 0)
	store.Get(tenant, "ns1", "s2", 0)
	store.Get(tenant, "ns1", "s20", 0)
	store.Get(tenant, "ns1", "s3", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 4, len(actions), "unexpected actions: %#v", actions)
	fakeClient.ClearActions()

	// Create a new pod that is refencing the first three secrets - those should
	// be invalidated.
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name2", s1))
	store.Get(tenant, "ns1", "s1", 0)
	store.Get(tenant, "ns1", "s10", 0)
	store.Get(tenant, "ns1", "s2", 0)
	store.Get(tenant, "ns1", "s20", 0)
	store.Get(tenant, "ns1", "s3", 0)
	actions = fakeClient.Actions()
	assert.Equal(t, 3, len(actions), "unexpected actions: %#v", actions)
	fakeClient.ClearActions()
}

func TestRegisterIdempotence(t *testing.T) {
	testRegisterIdempotence(t, metav1.TenantSystem)
}

func TestRegisterIdempotenceWithMultiTenancy(t *testing.T) {
	testRegisterIdempotence(t, "test-te")
}

func testRegisterIdempotence(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	fakeClock := clock.NewFakeClock(time.Now())
	store := newSecretStore(fakeClient, fakeClock, noObjectTTL, time.Minute)
	manager := newCacheBasedSecretManager(store)

	s1 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
	}

	refs := func(tenant, ns, name string) int {
		store.lock.Lock()
		defer store.lock.Unlock()
		item, ok := store.items[objectKey{tenant, ns, name, 0}]
		if !ok {
			return 0
		}
		return item.refCount
	}

	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	assert.Equal(t, 1, refs(tenant, "ns1", "s1"))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	assert.Equal(t, 1, refs(tenant, "ns1", "s1"))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name2", s1))
	assert.Equal(t, 2, refs(tenant, "ns1", "s1"))

	manager.UnregisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	assert.Equal(t, 1, refs(tenant, "ns1", "s1"))
	manager.UnregisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	assert.Equal(t, 1, refs(tenant, "ns1", "s1"))
	manager.UnregisterPod(podWithSecrets(tenant, "ns1", "name2", s1))
	assert.Equal(t, 0, refs(tenant, "ns1", "s1"))
}

func TestCacheRefcounts(t *testing.T) {
	testCacheRefcounts(t, metav1.TenantSystem)
}

func TestCacheRefcountsWithMultiTenancy(t *testing.T) {
	testCacheRefcounts(t, "test-te")
}

func testCacheRefcounts(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	fakeClock := clock.NewFakeClock(time.Now())
	store := newSecretStore(fakeClient, fakeClock, noObjectTTL, time.Minute)
	manager := newCacheBasedSecretManager(store)

	s1 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s1"}, envFromNames: []string{"s10"}},
			{envVarNames: []string{"s2"}},
			{envVarNames: []string{"s3"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name2", s1))
	s2 := secretsToAttach{
		imagePullSecretNames: []string{"s2"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s4"}},
			{envVarNames: []string{"s5"}, envFromNames: []string{"s50"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name2", s2))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name3", s2))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name4", s2))
	manager.UnregisterPod(podWithSecrets(tenant, "ns1", "name3", s2))
	s3 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s3"}, envFromNames: []string{"s30"}},
			{envVarNames: []string{"s5"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name5", s3))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name6", s3))
	s4 := secretsToAttach{
		imagePullSecretNames: []string{"s3"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s6"}},
			{envFromNames: []string{"s60"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name7", s4))
	manager.UnregisterPod(podWithSecrets(tenant, "ns1", "name7", s4))

	// Also check the Add + Update + Remove scenario.
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "other-name", s1))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "other-name", s2))
	manager.UnregisterPod(podWithSecrets(tenant, "ns1", "other-name", s2))

	s5 := secretsToAttach{
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s7"}},
			{envFromNames: []string{"s70"}},
		},
	}
	// Check the no-op update scenario
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "noop-pod", s5))
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "noop-pod", s5))

	// Now we have: 3 pods with s1, 2 pods with s2 and 2 pods with s3, 0 pods with s4.
	refs := func(tenant, ns, name string) int {
		store.lock.Lock()
		defer store.lock.Unlock()
		item, ok := store.items[objectKey{tenant, ns, name, 0}]
		if !ok {
			return 0
		}
		return item.refCount
	}
	assert.Equal(t, 3, refs(tenant, "ns1", "s1"))
	assert.Equal(t, 1, refs(tenant, "ns1", "s10"))
	assert.Equal(t, 3, refs(tenant, "ns1", "s2"))
	assert.Equal(t, 3, refs(tenant, "ns1", "s3"))
	assert.Equal(t, 2, refs(tenant, "ns1", "s30"))
	assert.Equal(t, 2, refs(tenant, "ns1", "s4"))
	assert.Equal(t, 4, refs(tenant, "ns1", "s5"))
	assert.Equal(t, 2, refs(tenant, "ns1", "s50"))
	assert.Equal(t, 0, refs(tenant, "ns1", "s6"))
	assert.Equal(t, 0, refs(tenant, "ns1", "s60"))
	assert.Equal(t, 1, refs(tenant, "ns1", "s7"))
	assert.Equal(t, 1, refs(tenant, "ns1", "s70"))
}

func TestCacheBasedSecretManager(t *testing.T) {
	testCacheBasedSecretManager(t, metav1.TenantSystem)
}

func TestCacheBasedSecretManagerWithMultiTenancy(t *testing.T) {
	testCacheBasedSecretManager(t, "test-te")
}

func testCacheBasedSecretManager(t *testing.T, tenant string) {
	fakeClient := &fake.Clientset{}
	store := newSecretStore(fakeClient, clock.RealClock{}, noObjectTTL, 0)
	manager := newCacheBasedSecretManager(store)

	// Create a pod with some secrets.
	s1 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s1"}},
			{envVarNames: []string{"s2"}},
			{envFromNames: []string{"s20"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s1))
	// Update the pod with a different secrets.
	s2 := secretsToAttach{
		imagePullSecretNames: []string{"s1"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s3"}},
			{envVarNames: []string{"s4"}},
			{envFromNames: []string{"s40"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns1", "name1", s2))
	// Create another pod, but with same secrets in different namespace.
	manager.RegisterPod(podWithSecrets(tenant, "ns2", "name2", s2))
	// Create and delete a pod with some other secrets.
	s3 := secretsToAttach{
		imagePullSecretNames: []string{"s5"},
		containerEnvSecrets: []envSecrets{
			{envVarNames: []string{"s6"}},
			{envFromNames: []string{"s60"}},
		},
	}
	manager.RegisterPod(podWithSecrets(tenant, "ns3", "name", s3))
	manager.UnregisterPod(podWithSecrets(tenant, "ns3", "name", s3))

	// We should have only: s1, s3 and s4 secrets in namespaces: ns1 and ns2.
	for _, ns := range []string{"ns1", "ns2", "ns3"} {
		for _, secret := range []string{"s1", "s2", "s3", "s4", "s5", "s6", "s20", "s40", "s50"} {
			shouldExist :=
				(secret == "s1" || secret == "s3" || secret == "s4" || secret == "s40") && (ns == "ns1" || ns == "ns2")
			checkObject(t, store, tenant, ns, secret, shouldExist)
		}
	}
}
