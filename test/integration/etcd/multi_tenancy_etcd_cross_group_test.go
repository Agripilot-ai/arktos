/*
Copyright 2020 Authors of Arktos.

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

package etcd

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/dynamic"
	"k8s.io/kubernetes/cmd/kube-apiserver/app/options"
)

// TestCrossGroupStorage tests to make sure that all objects stored in an expected location in etcd can be converted/read.
func TestCrossGroupStorageWithMultiTenancy(t *testing.T) {
	master := StartRealMasterOrDie(t, func(opts *options.ServerRunOptions) {
		// force enable all resources so we can check storage.
		// TODO: drop these once we stop allowing them to be served.
		opts.APIEnablement.RuntimeConfig["extensions/v1beta1/deployments"] = "true"
		opts.APIEnablement.RuntimeConfig["extensions/v1beta1/daemonsets"] = "true"
		opts.APIEnablement.RuntimeConfig["extensions/v1beta1/replicasets"] = "true"
		opts.APIEnablement.RuntimeConfig["extensions/v1beta1/podsecuritypolicies"] = "true"
		opts.APIEnablement.RuntimeConfig["extensions/v1beta1/networkpolicies"] = "true"
	})
	defer master.Cleanup()
	etcdStorageData := GetEtcdStorageDataWithMultiTenancy()

	crossGroupResources := map[schema.GroupVersionKind][]Resource{}

	master.Client.CoreV1().NamespacesWithMultiTenancy(testTenant).Create(&v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: testNamespace, Tenant: testTenant}})

	// Group by persisted GVK
	for _, resourceToPersist := range master.Resources {
		gvk := resourceToPersist.Mapping.GroupVersionKind

		// daemonset not allowed in user tenant; skip its checking in multi tenants
		if gvk.GroupKind().Kind == "DaemonSet" {
			continue
		}

		data, exists := etcdStorageData[resourceToPersist.Mapping.Resource]
		if !exists {
			continue
		}
		storageGVK := gvk
		if data.ExpectedGVK != nil {
			storageGVK = *data.ExpectedGVK
		}
		crossGroupResources[storageGVK] = append(crossGroupResources[storageGVK], resourceToPersist)
	}

	// Clear any without cross-group sources
	for gvk, resources := range crossGroupResources {
		groups := sets.NewString()
		for _, resource := range resources {
			groups.Insert(resource.Mapping.GroupVersionKind.Group)
		}
		if len(groups) < 2 {
			delete(crossGroupResources, gvk)
		}
	}

	if len(crossGroupResources) == 0 {
		// Sanity check
		t.Fatal("no cross-group resources found")
	}

	// Test all potential cross-group sources can be watched and fetched from all other sources
	for gvk, resources := range crossGroupResources {
		t.Run(gvk.String(), func(t *testing.T) {
			// use the first one to create the initial object
			resource := resources[0]

			// compute tenant/namespace
			tenant, namespace := "", ""
			if resource.Mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				tenant = testTenant
				namespace = testNamespace
			}

			if resource.Mapping.Scope.Name() == meta.RESTScopeNameTenant {
				tenant = testTenant
			}

			data := etcdStorageData[resource.Mapping.Resource]
			// create object
			resourceClient, obj, err := JSONToUnstructuredWithMultiTenancy(data.Stub, tenant, namespace, resource.Mapping, master.Dynamic)
			if err != nil {
				t.Fatal(err)
			}
			actual, err := resourceClient.Create(obj, metav1.CreateOptions{})
			if err != nil {
				t.Fatal(err)
			}
			name := actual.GetName()

			// Set up clients, versioned data, and watches for all versions
			var (
				clients       = map[schema.GroupVersionResource]dynamic.ResourceInterface{}
				versionedData = map[schema.GroupVersionResource]*unstructured.Unstructured{}
				watches       = map[schema.GroupVersionResource]watch.Interface{}
			)
			for _, resource := range resources {
				clients[resource.Mapping.Resource] = master.Dynamic.Resource(resource.Mapping.Resource).NamespaceWithMultiTenancy(namespace, tenant)
				versionedData[resource.Mapping.Resource], err = clients[resource.Mapping.Resource].Get(name, metav1.GetOptions{})
				if err != nil {
					t.Fatalf("error finding resource via %s: %v", resource.Mapping.Resource.GroupVersion().String(), err)
				}
				watches[resource.Mapping.Resource], err = clients[resource.Mapping.Resource].Watch(metav1.ListOptions{ResourceVersion: actual.GetResourceVersion()})
				if err != nil {
					t.Fatalf("error opening watch via %s: %v", resource.Mapping.Resource.GroupVersion().String(), err)
				}
			}

			for _, resource := range resources {
				// clear out the things cleared in etcd
				versioned := versionedData[resource.Mapping.Resource]
				versioned.SetResourceVersion("")
				versioned.SetSelfLink("")
				versionedJSON, err := versioned.MarshalJSON()
				if err != nil {
					t.Error(err)
					continue
				}

				// Update in etcd
				if _, err := master.KV.Put(context.Background(), data.ExpectedEtcdPath, string(versionedJSON)); err != nil {
					t.Error(err)
					continue
				}
				t.Logf("wrote %s to etcd", resource.Mapping.Resource.GroupVersion().String())

				// Ensure everyone gets a watch event with the right version
				for watchResource, watcher := range watches {
					select {
					case event, ok := <-watcher.ResultChan():
						if !ok {
							t.Fatalf("watch of %s closed in response to persisting %s", watchResource.GroupVersion().String(), resource.Mapping.Resource.GroupVersion().String())
						}
						if event.Type != watch.Modified {
							eventJSON, _ := json.Marshal(event)
							t.Errorf("unexpected watch event sent to watch of %s in response to persisting %s: %s", watchResource.GroupVersion().String(), resource.Mapping.Resource.GroupVersion().String(), string(eventJSON))
							continue
						}
						if event.Object.GetObjectKind().GroupVersionKind().GroupVersion() != watchResource.GroupVersion() {
							t.Errorf("unexpected group version object sent to watch of %s in response to persisting %s: %#v", watchResource.GroupVersion().String(), resource.Mapping.Resource.GroupVersion().String(), event.Object)
							continue
						}
						t.Logf("     received event for %s", watchResource.GroupVersion().String())
					case <-time.After(30 * time.Second):
						t.Errorf("timed out waiting for watch event for %s in response to persisting %s", watchResource.GroupVersion().String(), resource.Mapping.Resource.GroupVersion().String())
						continue
					}
				}

				// Ensure everyone can do a direct get and gets the right version
				for clientResource, client := range clients {
					obj, err := client.Get(name, metav1.GetOptions{})
					if err != nil {
						t.Errorf("error looking up %s after persisting %s", clientResource.GroupVersion().String(), resource.Mapping.Resource.GroupVersion().String())
						continue
					}
					if obj.GetObjectKind().GroupVersionKind().GroupVersion() != clientResource.GroupVersion() {
						t.Errorf("unexpected group version retrieved from %s after persisting %s: %#v", clientResource.GroupVersion().String(), resource.Mapping.Resource.GroupVersion().String(), obj)
						continue
					}
					t.Logf("     fetched object for %s", clientResource.GroupVersion().String())
				}
			}
		})
	}
}
