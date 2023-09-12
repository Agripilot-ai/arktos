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

package metadatainformer

import (
	"context"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/tools/cache"
)

func TestMetadataSharedInformerFactoryWithMultiTenancy(t *testing.T) {
	scenarios := []struct {
		name        string
		existingObj *metav1.PartialObjectMetadata
		gvr         schema.GroupVersionResource
		ns          string
		tenant      string
		trigger     func(gvr schema.GroupVersionResource, te, ns string, fakeClient *fake.FakeMetadataClient, testObject *metav1.PartialObjectMetadata) *metav1.PartialObjectMetadata
		handler     func(rcvCh chan<- *metav1.PartialObjectMetadata) *cache.ResourceEventHandlerFuncs
	}{
		// scenario 1
		{
			name:   "scenario 1: test if adding an object triggers AddFunc",
			ns:     "ns-foo",
			tenant: "te-foo",
			gvr:    schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "deployments"},
			trigger: func(gvr schema.GroupVersionResource, te, ns string, fakeClient *fake.FakeMetadataClient, _ *metav1.PartialObjectMetadata) *metav1.PartialObjectMetadata {
				testObject := newPartialObjectMetadataWithMultiTenancy("extensions/v1beta1", "Deployment", "ns-foo", "name-foo", "te-foo")
				createdObj, err := fakeClient.Resource(gvr).NamespaceWithMultiTenancy(ns, te).(fake.MetadataClient).CreateFake(testObject, metav1.CreateOptions{})
				if err != nil {
					t.Error(err)
				}
				return createdObj
			},
			handler: func(rcvCh chan<- *metav1.PartialObjectMetadata) *cache.ResourceEventHandlerFuncs {
				return &cache.ResourceEventHandlerFuncs{
					AddFunc: func(obj interface{}) {
						rcvCh <- obj.(*metav1.PartialObjectMetadata)
					},
				}
			},
		},

		// scenario 2
		{
			name:        "scenario 2: tests if updating an object triggers UpdateFunc",
			ns:          "ns-foo",
			tenant:      "te-foo",
			gvr:         schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "deployments"},
			existingObj: newPartialObjectMetadataWithMultiTenancy("extensions/v1beta1", "Deployment", "ns-foo", "name-foo", "te-foo"),
			trigger: func(gvr schema.GroupVersionResource, te, ns string, fakeClient *fake.FakeMetadataClient, testObject *metav1.PartialObjectMetadata) *metav1.PartialObjectMetadata {
				if testObject.Annotations == nil {
					testObject.Annotations = make(map[string]string)
				}
				testObject.Annotations["test"] = "updatedName"
				updatedObj, err := fakeClient.Resource(gvr).NamespaceWithMultiTenancy(ns, te).(fake.MetadataClient).UpdateFake(testObject, metav1.UpdateOptions{})
				if err != nil {
					t.Error(err)
				}
				return updatedObj
			},
			handler: func(rcvCh chan<- *metav1.PartialObjectMetadata) *cache.ResourceEventHandlerFuncs {
				return &cache.ResourceEventHandlerFuncs{
					UpdateFunc: func(old, updated interface{}) {
						rcvCh <- updated.(*metav1.PartialObjectMetadata)
					},
				}
			},
		},

		// scenario 3
		{
			name:        "scenario 3: test if deleting an object triggers DeleteFunc",
			ns:          "ns-foo",
			tenant:      "te-foo",
			gvr:         schema.GroupVersionResource{Group: "extensions", Version: "v1beta1", Resource: "deployments"},
			existingObj: newPartialObjectMetadataWithMultiTenancy("extensions/v1beta1", "Deployment", "ns-foo", "name-foo", "te-foo"),
			trigger: func(gvr schema.GroupVersionResource, te, ns string, fakeClient *fake.FakeMetadataClient, testObject *metav1.PartialObjectMetadata) *metav1.PartialObjectMetadata {
				err := fakeClient.Resource(gvr).NamespaceWithMultiTenancy(ns, te).Delete(testObject.GetName(), &metav1.DeleteOptions{})
				if err != nil {
					t.Error(err)
				}
				return testObject
			},
			handler: func(rcvCh chan<- *metav1.PartialObjectMetadata) *cache.ResourceEventHandlerFuncs {
				return &cache.ResourceEventHandlerFuncs{
					DeleteFunc: func(obj interface{}) {
						rcvCh <- obj.(*metav1.PartialObjectMetadata)
					},
				}
			},
		},
	}

	for _, ts := range scenarios {
		t.Run(ts.name, func(t *testing.T) {
			// test data
			timeout := time.Duration(3 * time.Second)
			ctx, cancel := context.WithTimeout(context.Background(), timeout)
			defer cancel()
			scheme := runtime.NewScheme()
			metav1.AddMetaToScheme(scheme)
			informerReciveObjectCh := make(chan *metav1.PartialObjectMetadata, 1)
			objs := []runtime.Object{}
			if ts.existingObj != nil {
				objs = append(objs, ts.existingObj)
			}
			fakeClient := fake.NewSimpleMetadataClient(scheme, objs...)
			target := NewSharedInformerFactoryWithMultitenancy(fakeClient, ts.tenant, 0)

			// act
			informerListerForGvr := target.ForResource(ts.gvr)
			informerListerForGvr.Informer().AddEventHandler(ts.handler(informerReciveObjectCh))
			target.Start(ctx.Done())
			if synced := target.WaitForCacheSync(ctx.Done()); !synced[ts.gvr] {
				t.Fatalf("informer for %s hasn't synced", ts.gvr)
			}

			testObject := ts.trigger(ts.gvr, ts.tenant, ts.ns, fakeClient, ts.existingObj)
			select {
			case objFromInformer := <-informerReciveObjectCh:
				if !equality.Semantic.DeepEqual(testObject, objFromInformer) {
					t.Fatalf("%v", diff.ObjectDiff(testObject, objFromInformer))
				}
			case <-ctx.Done():
				t.Errorf("tested informer haven't received an object, waited %v", timeout)
			}
		})
	}
}