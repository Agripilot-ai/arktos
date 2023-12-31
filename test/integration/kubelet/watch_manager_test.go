/*
Copyright 2019 The Kubernetes Authors.
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

package kubelet

import (
	"fmt"
	"sync"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	kubeapiservertesting "k8s.io/kubernetes/cmd/kube-apiserver/app/testing"
	"k8s.io/kubernetes/pkg/kubelet/util/manager"
	"k8s.io/kubernetes/test/integration/framework"
)

func TestWatchBasedManager(t *testing.T) {
	testWatchBasedManager(t, metav1.TenantSystem)
}

func TestWatchBasedManagerWithMultiTenancy(t *testing.T) {
	testWatchBasedManager(t, "test-te")
}

func testWatchBasedManager(t *testing.T, tenant string) {
	testNamespace := "test-watch-based-manager"
	server := kubeapiservertesting.StartTestServerOrDie(t, nil, nil, framework.SharedEtcd())
	defer server.TearDownFn()

	for _, config := range server.ClientConfig.GetAllConfigs() {
		config.QPS = 10000
	}
	client, err := kubernetes.NewForConfig(server.ClientConfig)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tenant != metav1.TenantSystem {
		if _, err := client.CoreV1().Tenants().Create((&v1.Tenant{ObjectMeta: metav1.ObjectMeta{Name: tenant}, Spec: v1.TenantSpec{StorageClusterId: "1"}})); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.CoreV1().NamespacesWithMultiTenancy(tenant).Create((&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace}})); err != nil {
		t.Fatal(err)
	}

	listObj := func(tenant, namespace string, _ int, options metav1.ListOptions) (runtime.Object, error) {
		return client.CoreV1().SecretsWithMultiTenancy(namespace, tenant).List(options)
	}
	watchObj := func(tenant, namespace string, _ int, options metav1.ListOptions) (watch.Interface, error) {
		return client.CoreV1().SecretsWithMultiTenancy(namespace, tenant).Watch(options)
	}
	newObj := func() runtime.Object { return &v1.Secret{} }
	store := manager.NewObjectCache(listObj, watchObj, newObj, schema.GroupResource{Group: "v1", Resource: "secrets"})

	// create 1000 secrets in parallel
	t.Log(time.Now(), "creating 1000 secrets")
	wg := sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				name := fmt.Sprintf("s%d", i*100+j)
				if _, err := client.CoreV1().SecretsWithMultiTenancy(testNamespace, tenant).Create(&v1.Secret{ObjectMeta: metav1.ObjectMeta{Name: name}}); err != nil {
					t.Fatal(err)
				}
			}
			fmt.Print(".")
		}(i)
	}
	wg.Wait()
	t.Log(time.Now(), "finished creating 1000 secrets")

	// fetch all secrets
	wg = sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				name := fmt.Sprintf("s%d", i*100+j)
				start := time.Now()
				store.AddReference(tenant, testNamespace, name, 0)
				err := wait.PollImmediate(10*time.Millisecond, 10*time.Second, func() (bool, error) {
					obj, err := store.Get(tenant, testNamespace, name, 0)
					if err != nil {
						t.Logf("failed on %s, retrying: %v", name, err)
						return false, nil
					}
					if obj.(*v1.Secret).Name != name {
						return false, fmt.Errorf("wrong object: %v", obj.(*v1.Secret).Name)
					}
					return true, nil
				})
				if err != nil {
					t.Fatalf("failed on %s: %v", name, err)
				}
				if d := time.Since(start); d > time.Second {
					t.Logf("%s took %v", name, d)
				}
			}
		}(i)
	}
	wg.Wait()
}
