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

// Package token implements a manager of serviceaccount tokens for pods running
// on the node.
package token

import (
	"errors"
	"fmt"
	"sync"
	"time"

	authenticationv1 "k8s.io/api/authentication/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/clock"
	"k8s.io/apimachinery/pkg/util/wait"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog"
	"k8s.io/kubernetes/pkg/kubelet/kubeclientmanager"
)

const (
	maxTTL   = 24 * time.Hour
	gcPeriod = time.Minute
)

// NewManager returns a new token manager.
func NewManager(clients []clientset.Interface) *Manager {
	m := &Manager{
		getToken: func(tenant, name, namespace string, ownerPodUID types.UID, tr *authenticationv1.TokenRequest) (*authenticationv1.TokenRequest, error) {
			if len(clients) == 0 {
				return nil, errors.New("cannot use TokenManager when kubelet is in standalone mode")
			}
			idx := kubeclientmanager.ClientManager.PickClient(ownerPodUID)
			return clients[idx].CoreV1().ServiceAccountsWithMultiTenancy(namespace, tenant).CreateToken(name, tr)
		},
		cache: make(map[string]*authenticationv1.TokenRequest),
		clock: clock.RealClock{},
	}
	go wait.Forever(m.cleanup, gcPeriod)
	return m
}

// Manager manages service account tokens for pods.
type Manager struct {

	// cacheMutex guards the cache
	cacheMutex sync.RWMutex
	cache      map[string]*authenticationv1.TokenRequest

	// mocked for testing
	getToken func(tenant, name, namespace string, ownerPodUID types.UID, tr *authenticationv1.TokenRequest) (*authenticationv1.TokenRequest, error)
	clock    clock.Clock
}

// GetServiceAccountToken gets a service account token for a pod from cache or
// from the TokenRequest API. This process is as follows:
// * Check the cache for the current token request.
// * If the token exists and does not require a refresh, return the current token.
// * Attempt to refresh the token.
// * If the token is refreshed successfully, save it in the cache and return the token.
// * If refresh fails and the old token is still valid, log an error and return the old token.
// * If refresh fails and the old token is no longer valid, return an error
func (m *Manager) GetServiceAccountToken(tenant, namespace, name string, ownerPodUID types.UID, tr *authenticationv1.TokenRequest) (*authenticationv1.TokenRequest, error) {
	key := keyFunc(name, namespace, tr)

	ctr, ok := m.get(key)

	if ok && !m.requiresRefresh(ctr) {
		return ctr, nil
	}

	tr, err := m.getToken(tenant, name, namespace, ownerPodUID, tr)
	if err != nil {
		switch {
		case !ok:
			return nil, fmt.Errorf("failed to fetch token: %v", err)
		case m.expired(ctr):
			return nil, fmt.Errorf("token %s expired and refresh failed: %v", key, err)
		default:
			klog.Errorf("couldn't update token %s: %v", key, err)
			return ctr, nil
		}
	}

	m.set(key, tr)
	return tr, nil
}

// DeleteServiceAccountToken should be invoked when pod got deleted. It simply
// clean token manager cache.
func (m *Manager) DeleteServiceAccountToken(podUID types.UID) {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()
	for k, tr := range m.cache {
		if tr.Spec.BoundObjectRef.UID == podUID {
			delete(m.cache, k)
		}
	}
}

func (m *Manager) cleanup() {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()
	for k, tr := range m.cache {
		if m.expired(tr) {
			delete(m.cache, k)
		}
	}
}

func (m *Manager) get(key string) (*authenticationv1.TokenRequest, bool) {
	m.cacheMutex.RLock()
	defer m.cacheMutex.RUnlock()
	ctr, ok := m.cache[key]
	return ctr, ok
}

func (m *Manager) set(key string, tr *authenticationv1.TokenRequest) {
	m.cacheMutex.Lock()
	defer m.cacheMutex.Unlock()
	m.cache[key] = tr
}

func (m *Manager) expired(t *authenticationv1.TokenRequest) bool {
	return m.clock.Now().After(t.Status.ExpirationTimestamp.Time)
}

// requiresRefresh returns true if the token is older than 80% of its total
// ttl, or if the token is older than 24 hours.
func (m *Manager) requiresRefresh(tr *authenticationv1.TokenRequest) bool {
	if tr.Spec.ExpirationSeconds == nil {
		klog.Errorf("expiration seconds was nil for tr: %#v", tr)
		return false
	}
	now := m.clock.Now()
	exp := tr.Status.ExpirationTimestamp.Time
	iat := exp.Add(-1 * time.Duration(*tr.Spec.ExpirationSeconds) * time.Second)

	if now.After(iat.Add(maxTTL)) {
		return true
	}
	// Require a refresh if within 20% of the TTL from the expiration time.
	if now.After(exp.Add(-1 * time.Duration((*tr.Spec.ExpirationSeconds*20)/100) * time.Second)) {
		return true
	}
	return false
}

// keys should be nonconfidential and safe to log
func keyFunc(name, namespace string, tr *authenticationv1.TokenRequest) string {
	var exp int64
	if tr.Spec.ExpirationSeconds != nil {
		exp = *tr.Spec.ExpirationSeconds
	}

	var ref authenticationv1.BoundObjectReference
	if tr.Spec.BoundObjectRef != nil {
		ref = *tr.Spec.BoundObjectRef
	}

	return fmt.Sprintf("%q/%q/%#v/%#v/%#v", name, namespace, tr.Spec.Audiences, exp, ref)
}
