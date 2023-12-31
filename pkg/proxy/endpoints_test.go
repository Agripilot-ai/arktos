/*
Copyright 2017 The Kubernetes Authors.
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

package proxy

import (
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
)

const tpId = 0

func (proxier *FakeProxier) addEndpoints(endpoints *v1.Endpoints, tenantPartitionId int) {
	proxier.endpointsChanges.Update(nil, endpoints, tenantPartitionId)
}

func (proxier *FakeProxier) updateEndpoints(oldEndpoints, endpoints *v1.Endpoints, tenantPartitionId int) {
	proxier.endpointsChanges.Update(oldEndpoints, endpoints, tenantPartitionId)
}

func (proxier *FakeProxier) deleteEndpoints(endpoints *v1.Endpoints, tenantPartitionId int) {
	proxier.endpointsChanges.Update(endpoints, nil, tenantPartitionId)
}

func TestGetLocalEndpointIPs(t *testing.T) {
	testCases := []struct {
		endpointsMap EndpointsMap
		expected     map[types.NamespacednameWithTenantSource]sets.String
	}{{
		// Case[0]: nothing
		endpointsMap: EndpointsMap{},
		expected:     map[types.NamespacednameWithTenantSource]sets.String{},
	}, {
		// Case[1]: unnamed port
		endpointsMap: EndpointsMap{
			makeServicePortName("te", "ns1", "ep1", ""): []Endpoint{
				&BaseEndpointInfo{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expected: map[types.NamespacednameWithTenantSource]sets.String{},
	}, {
		// Case[2]: unnamed port local
		endpointsMap: EndpointsMap{
			makeServicePortName("te", "ns1", "ep1", ""): []Endpoint{
				&BaseEndpointInfo{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
		},
		expected: map[types.NamespacednameWithTenantSource]sets.String{
			{Tenant: "te", Namespace: "ns1", Name: "ep1"}: sets.NewString("1.1.1.1"),
		},
	}, {
		// Case[3]: named local and non-local ports for the same IP.
		endpointsMap: EndpointsMap{
			makeServicePortName("te", "ns1", "ep1", "p11"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "1.1.1.1:11", IsLocal: false},
				&BaseEndpointInfo{Endpoint: "1.1.1.2:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "1.1.1.1:12", IsLocal: false},
				&BaseEndpointInfo{Endpoint: "1.1.1.2:12", IsLocal: true},
			},
		},
		expected: map[types.NamespacednameWithTenantSource]sets.String{
			{Tenant: "te", Namespace: "ns1", Name: "ep1"}: sets.NewString("1.1.1.2"),
		},
	}, {
		// Case[4]: named local and non-local ports for different IPs.
		endpointsMap: EndpointsMap{
			makeServicePortName("te", "ns1", "ep1", "p11"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
			makeServicePortName("te", "ns2", "ep2", "p22"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "2.2.2.2:22", IsLocal: true},
				&BaseEndpointInfo{Endpoint: "2.2.2.22:22", IsLocal: true},
			},
			makeServicePortName("te", "ns2", "ep2", "p23"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "2.2.2.3:23", IsLocal: true},
			},
			makeServicePortName("te", "ns4", "ep4", "p44"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "4.4.4.4:44", IsLocal: true},
				&BaseEndpointInfo{Endpoint: "4.4.4.5:44", IsLocal: false},
			},
			makeServicePortName("te", "ns4", "ep4", "p45"): []Endpoint{
				&BaseEndpointInfo{Endpoint: "4.4.4.6:45", IsLocal: true},
			},
		},
		expected: map[types.NamespacednameWithTenantSource]sets.String{
			{Tenant: "te", Namespace: "ns2", Name: "ep2"}: sets.NewString("2.2.2.2", "2.2.2.22", "2.2.2.3"),
			{Tenant: "te", Namespace: "ns4", Name: "ep4"}: sets.NewString("4.4.4.4", "4.4.4.6"),
		},
	}}

	for tci, tc := range testCases {
		// outputs
		localIPs := tc.endpointsMap.getLocalEndpointIPs()

		if !reflect.DeepEqual(localIPs, tc.expected) {
			t.Errorf("[%d] expected %#v, got %#v", tci, tc.expected, localIPs)
		}
	}
}

func makeTestEndpoints(tenant, namespace, name string, eptFunc func(*v1.Endpoints)) *v1.Endpoints {
	ept := &v1.Endpoints{
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Tenant:      tenant,
			Annotations: make(map[string]string),
		},
	}
	eptFunc(ept)
	return ept
}

// This is a coarse test, but it offers some modicum of confidence as the code is evolved.
func TestEndpointsToEndpointsMap(t *testing.T) {
	epTracker := NewEndpointChangeTracker("test-hostname", nil, nil, nil)

	trueVal := true
	falseVal := false

	testCases := []struct {
		desc         string
		newEndpoints *v1.Endpoints
		expected     map[ServicePortName][]*BaseEndpointInfo
		isIPv6Mode   *bool
	}{
		{
			desc:         "nothing",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {}),
			expected:     map[ServicePortName][]*BaseEndpointInfo{},
		},
		{
			desc: "no changes, unnamed port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}},
						Ports: []v1.EndpointPort{{
							Name: "",
							Port: 11,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", ""): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
				},
			},
		},
		{
			desc: "no changes, named port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}},
						Ports: []v1.EndpointPort{{
							Name: "port",
							Port: 11,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "port"): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
				},
			},
		},
		{
			desc: "new port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}},
						Ports: []v1.EndpointPort{{
							Port: 11,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", ""): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
				},
			},
		},
		{
			desc:         "remove port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {}),
			expected:     map[ServicePortName][]*BaseEndpointInfo{},
		},
		{
			desc: "new IP and port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}, {
							IP: "2.2.2.2",
						}},
						Ports: []v1.EndpointPort{{
							Name: "p1",
							Port: 11,
						}, {
							Name: "p2",
							Port: 22,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "p1"): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
					{Endpoint: "2.2.2.2:11", IsLocal: false},
				},
				makeServicePortName("te", "ns1", "ep1", "p2"): {
					{Endpoint: "1.1.1.1:22", IsLocal: false},
					{Endpoint: "2.2.2.2:22", IsLocal: false},
				},
			},
		},
		{
			desc: "remove IP and port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}},
						Ports: []v1.EndpointPort{{
							Name: "p1",
							Port: 11,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "p1"): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
				},
			},
		},
		{
			desc: "rename port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}},
						Ports: []v1.EndpointPort{{
							Name: "p2",
							Port: 11,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "p2"): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
				},
			},
		},
		{
			desc: "renumber port",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}},
						Ports: []v1.EndpointPort{{
							Name: "p1",
							Port: 22,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "p1"): {
					{Endpoint: "1.1.1.1:22", IsLocal: false},
				},
			},
		},
		{
			desc: "should omit IPv6 address in IPv4 mode",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}, {
							IP: "2001:db8:85a3:0:0:8a2e:370:7334",
						}},
						Ports: []v1.EndpointPort{{
							Name: "p1",
							Port: 11,
						}, {
							Name: "p2",
							Port: 22,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "p1"): {
					{Endpoint: "1.1.1.1:11", IsLocal: false},
				},
				makeServicePortName("te", "ns1", "ep1", "p2"): {
					{Endpoint: "1.1.1.1:22", IsLocal: false},
				},
			},
			isIPv6Mode: &falseVal,
		},
		{
			desc: "should omit IPv4 address in IPv6 mode",
			newEndpoints: makeTestEndpoints("te", "ns1", "ep1", func(ept *v1.Endpoints) {
				ept.Subsets = []v1.EndpointSubset{
					{
						Addresses: []v1.EndpointAddress{{
							IP: "1.1.1.1",
						}, {
							IP: "2001:db8:85a3:0:0:8a2e:370:7334",
						}},
						Ports: []v1.EndpointPort{{
							Name: "p1",
							Port: 11,
						}, {
							Name: "p2",
							Port: 22,
						}},
					},
				}
			}),
			expected: map[ServicePortName][]*BaseEndpointInfo{
				makeServicePortName("te", "ns1", "ep1", "p1"): {
					{Endpoint: "[2001:db8:85a3:0:0:8a2e:370:7334]:11", IsLocal: false},
				},
				makeServicePortName("te", "ns1", "ep1", "p2"): {
					{Endpoint: "[2001:db8:85a3:0:0:8a2e:370:7334]:22", IsLocal: false},
				},
			},
			isIPv6Mode: &trueVal,
		},
	}

	for _, tc := range testCases {
		epTracker.isIPv6Mode = tc.isIPv6Mode
		// outputs
		newEndpoints := epTracker.endpointsToEndpointsMap(tc.newEndpoints, tpId)

		if len(newEndpoints) != len(tc.expected) {
			t.Errorf("[%s] expected %d new, got %d: %v", tc.desc, len(tc.expected), len(newEndpoints), spew.Sdump(newEndpoints))
		}
		for x := range tc.expected {
			if len(newEndpoints[x]) != len(tc.expected[x]) {
				t.Errorf("[%s] expected %d endpoints for %v, got %d", tc.desc, len(tc.expected[x]), x, len(newEndpoints[x]))
			} else {
				for i := range newEndpoints[x] {
					ep := newEndpoints[x][i].(*BaseEndpointInfo)
					if *ep != *(tc.expected[x][i]) {
						t.Errorf("[%s] expected new[%v][%d] to be %v, got %v", tc.desc, x, i, tc.expected[x][i], *ep)
					}
				}
			}
		}
	}
}

func TestUpdateEndpointsMap(t *testing.T) {
	var nodeName = testHostname

	emptyEndpoint := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{}
	}
	unnamedPort := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Port: 11,
			}},
		}}
	}
	unnamedPortLocal := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP:       "1.1.1.1",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Port: 11,
			}},
		}}
	}
	namedPortLocal := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP:       "1.1.1.1",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}},
		}}
	}
	namedPort := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}},
		}}
	}
	namedPortRenamed := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11-2",
				Port: 11,
			}},
		}}
	}
	namedPortRenumbered := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 22,
			}},
		}}
	}
	namedPortsLocalNoLocal := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}, {
				IP:       "1.1.1.2",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}, {
				Name: "p12",
				Port: 12,
			}},
		}}
	}
	multipleSubsets := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.2",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p12",
				Port: 12,
			}},
		}}
	}
	multipleSubsetsWithLocal := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP:       "1.1.1.2",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p12",
				Port: 12,
			}},
		}}
	}
	multipleSubsetsMultiplePortsLocal := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP:       "1.1.1.1",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}, {
				Name: "p12",
				Port: 12,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.3",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p13",
				Port: 13,
			}},
		}}
	}
	multipleSubsetsIPsPorts1 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}, {
				IP:       "1.1.1.2",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}, {
				Name: "p12",
				Port: 12,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.3",
			}, {
				IP:       "1.1.1.4",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p13",
				Port: 13,
			}, {
				Name: "p14",
				Port: 14,
			}},
		}}
	}
	multipleSubsetsIPsPorts2 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "2.2.2.1",
			}, {
				IP:       "2.2.2.2",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p21",
				Port: 21,
			}, {
				Name: "p22",
				Port: 22,
			}},
		}}
	}
	complexBefore1 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}},
		}}
	}
	complexBefore2 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP:       "2.2.2.2",
				NodeName: &nodeName,
			}, {
				IP:       "2.2.2.22",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p22",
				Port: 22,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP:       "2.2.2.3",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p23",
				Port: 23,
			}},
		}}
	}
	complexBefore4 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP:       "4.4.4.4",
				NodeName: &nodeName,
			}, {
				IP:       "4.4.4.5",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p44",
				Port: 44,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP:       "4.4.4.6",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p45",
				Port: 45,
			}},
		}}
	}
	complexAfter1 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.1",
			}, {
				IP: "1.1.1.11",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p11",
				Port: 11,
			}},
		}, {
			Addresses: []v1.EndpointAddress{{
				IP: "1.1.1.2",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p12",
				Port: 12,
			}, {
				Name: "p122",
				Port: 122,
			}},
		}}
	}
	complexAfter3 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP: "3.3.3.3",
			}},
			Ports: []v1.EndpointPort{{
				Name: "p33",
				Port: 33,
			}},
		}}
	}
	complexAfter4 := func(ept *v1.Endpoints) {
		ept.Subsets = []v1.EndpointSubset{{
			Addresses: []v1.EndpointAddress{{
				IP:       "4.4.4.4",
				NodeName: &nodeName,
			}},
			Ports: []v1.EndpointPort{{
				Name: "p44",
				Port: 44,
			}},
		}}
	}

	testCases := []struct {
		// previousEndpoints and currentEndpoints are used to call appropriate
		// handlers OnEndpoints* (based on whether corresponding values are nil
		// or non-nil) and must be of equal length.
		previousEndpoints         []*v1.Endpoints
		currentEndpoints          []*v1.Endpoints
		oldEndpoints              map[ServicePortName][]*BaseEndpointInfo
		expectedResult            map[ServicePortName][]*BaseEndpointInfo
		expectedStaleEndpoints    []ServiceEndpoint
		expectedStaleServiceNames map[ServicePortName]bool
		expectedHealthchecks      map[types.NamespacednameWithTenantSource]int
	}{{
		// Case[0]: nothing
		oldEndpoints:              map[ServicePortName][]*BaseEndpointInfo{},
		expectedResult:            map[ServicePortName][]*BaseEndpointInfo{},
		expectedStaleEndpoints:    []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[1]: no change, unnamed port
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", unnamedPort),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", unnamedPort),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", ""): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", ""): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedStaleEndpoints:    []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[2]: no change, named port, local
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPortLocal),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPortLocal),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
		},
		expectedStaleEndpoints:    []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns1", "ep1"): 1,
		},
	}, {
		// Case[3]: no change, multiple subsets
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsets),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsets),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.2:12", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.2:12", IsLocal: false},
			},
		},
		expectedStaleEndpoints:    []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[4]: no change, multiple subsets, multiple ports, local
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsetsMultiplePortsLocal),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsetsMultiplePortsLocal),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.1:12", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p13"): {
				{Endpoint: "1.1.1.3:13", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.1:12", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p13"): {
				{Endpoint: "1.1.1.3:13", IsLocal: false},
			},
		},
		expectedStaleEndpoints:    []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns1", "ep1"): 1,
		},
	}, {
		// Case[5]: no change, multiple endpoints, subsets, IPs, and ports
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsetsIPsPorts1),
			makeTestEndpoints("te", "ns2", "ep2", multipleSubsetsIPsPorts2),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsetsIPsPorts1),
			makeTestEndpoints("te", "ns2", "ep2", multipleSubsetsIPsPorts2),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
				{Endpoint: "1.1.1.2:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.1:12", IsLocal: false},
				{Endpoint: "1.1.1.2:12", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p13"): {
				{Endpoint: "1.1.1.3:13", IsLocal: false},
				{Endpoint: "1.1.1.4:13", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p14"): {
				{Endpoint: "1.1.1.3:14", IsLocal: false},
				{Endpoint: "1.1.1.4:14", IsLocal: true},
			},
			makeServicePortName("te", "ns2", "ep2", "p21"): {
				{Endpoint: "2.2.2.1:21", IsLocal: false},
				{Endpoint: "2.2.2.2:21", IsLocal: true},
			},
			makeServicePortName("te", "ns2", "ep2", "p22"): {
				{Endpoint: "2.2.2.1:22", IsLocal: false},
				{Endpoint: "2.2.2.2:22", IsLocal: true},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
				{Endpoint: "1.1.1.2:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.1:12", IsLocal: false},
				{Endpoint: "1.1.1.2:12", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p13"): {
				{Endpoint: "1.1.1.3:13", IsLocal: false},
				{Endpoint: "1.1.1.4:13", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p14"): {
				{Endpoint: "1.1.1.3:14", IsLocal: false},
				{Endpoint: "1.1.1.4:14", IsLocal: true},
			},
			makeServicePortName("te", "ns2", "ep2", "p21"): {
				{Endpoint: "2.2.2.1:21", IsLocal: false},
				{Endpoint: "2.2.2.2:21", IsLocal: true},
			},
			makeServicePortName("te", "ns2", "ep2", "p22"): {
				{Endpoint: "2.2.2.1:22", IsLocal: false},
				{Endpoint: "2.2.2.2:22", IsLocal: true},
			},
		},
		expectedStaleEndpoints:    []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns1", "ep1"): 2,
			makeNSN("te", "ns2", "ep2"): 1,
		},
	}, {
		// Case[6]: add an Endpoints
		previousEndpoints: []*v1.Endpoints{
			nil,
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", unnamedPortLocal),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", ""): {
				{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{
			makeServicePortName("te", "ns1", "ep1", ""): true,
		},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns1", "ep1"): 1,
		},
	}, {
		// Case[7]: remove an Endpoints
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", unnamedPortLocal),
		},
		currentEndpoints: []*v1.Endpoints{
			nil,
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", ""): {
				{Endpoint: "1.1.1.1:11", IsLocal: true},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{},
		expectedStaleEndpoints: []ServiceEndpoint{{
			Endpoint:        "1.1.1.1:11",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", ""),
		}},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[8]: add an IP and port
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPort),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPortsLocalNoLocal),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
				{Endpoint: "1.1.1.2:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.1:12", IsLocal: false},
				{Endpoint: "1.1.1.2:12", IsLocal: true},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{
			makeServicePortName("te", "ns1", "ep1", "p12"): true,
		},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns1", "ep1"): 1,
		},
	}, {
		// Case[9]: remove an IP and port
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPortsLocalNoLocal),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPort),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
				{Endpoint: "1.1.1.2:11", IsLocal: true},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.1:12", IsLocal: false},
				{Endpoint: "1.1.1.2:12", IsLocal: true},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{{
			Endpoint:        "1.1.1.2:11",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", "p11"),
		}, {
			Endpoint:        "1.1.1.1:12",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", "p12"),
		}, {
			Endpoint:        "1.1.1.2:12",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", "p12"),
		}},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[10]: add a subset
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPort),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsetsWithLocal),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.2:12", IsLocal: true},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{
			makeServicePortName("te", "ns1", "ep1", "p12"): true,
		},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns1", "ep1"): 1,
		},
	}, {
		// Case[11]: remove a subset
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", multipleSubsets),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPort),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.2:12", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{{
			Endpoint:        "1.1.1.2:12",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", "p12"),
		}},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[12]: rename a port
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPort),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPortRenamed),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11-2"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{{
			Endpoint:        "1.1.1.1:11",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", "p11"),
		}},
		expectedStaleServiceNames: map[ServicePortName]bool{
			makeServicePortName("te", "ns1", "ep1", "p11-2"): true,
		},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[13]: renumber a port
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPort),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", namedPortRenumbered),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:22", IsLocal: false},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{{
			Endpoint:        "1.1.1.1:11",
			ServicePortName: makeServicePortName("te", "ns1", "ep1", "p11"),
		}},
		expectedStaleServiceNames: map[ServicePortName]bool{},
		expectedHealthchecks:      map[types.NamespacednameWithTenantSource]int{},
	}, {
		// Case[14]: complex add and remove
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", complexBefore1),
			makeTestEndpoints("te", "ns2", "ep2", complexBefore2),
			nil,
			makeTestEndpoints("te", "ns4", "ep4", complexBefore4),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", complexAfter1),
			nil,
			makeTestEndpoints("te", "ns3", "ep3", complexAfter3),
			makeTestEndpoints("te", "ns4", "ep4", complexAfter4),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
			makeServicePortName("te", "ns2", "ep2", "p22"): {
				{Endpoint: "2.2.2.2:22", IsLocal: true},
				{Endpoint: "2.2.2.22:22", IsLocal: true},
			},
			makeServicePortName("te", "ns2", "ep2", "p23"): {
				{Endpoint: "2.2.2.3:23", IsLocal: true},
			},
			makeServicePortName("te", "ns4", "ep4", "p44"): {
				{Endpoint: "4.4.4.4:44", IsLocal: true},
				{Endpoint: "4.4.4.5:44", IsLocal: true},
			},
			makeServicePortName("te", "ns4", "ep4", "p45"): {
				{Endpoint: "4.4.4.6:45", IsLocal: true},
			},
		},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", "p11"): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
				{Endpoint: "1.1.1.11:11", IsLocal: false},
			},
			makeServicePortName("te", "ns1", "ep1", "p12"): {
				{Endpoint: "1.1.1.2:12", IsLocal: false},
			},
			makeServicePortName("te", "ns1", "ep1", "p122"): {
				{Endpoint: "1.1.1.2:122", IsLocal: false},
			},
			makeServicePortName("te", "ns3", "ep3", "p33"): {
				{Endpoint: "3.3.3.3:33", IsLocal: false},
			},
			makeServicePortName("te", "ns4", "ep4", "p44"): {
				{Endpoint: "4.4.4.4:44", IsLocal: true},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{{
			Endpoint:        "2.2.2.2:22",
			ServicePortName: makeServicePortName("te", "ns2", "ep2", "p22"),
		}, {
			Endpoint:        "2.2.2.22:22",
			ServicePortName: makeServicePortName("te", "ns2", "ep2", "p22"),
		}, {
			Endpoint:        "2.2.2.3:23",
			ServicePortName: makeServicePortName("te", "ns2", "ep2", "p23"),
		}, {
			Endpoint:        "4.4.4.5:44",
			ServicePortName: makeServicePortName("te", "ns4", "ep4", "p44"),
		}, {
			Endpoint:        "4.4.4.6:45",
			ServicePortName: makeServicePortName("te", "ns4", "ep4", "p45"),
		}},
		expectedStaleServiceNames: map[ServicePortName]bool{
			makeServicePortName("te", "ns1", "ep1", "p12"):  true,
			makeServicePortName("te", "ns1", "ep1", "p122"): true,
			makeServicePortName("te", "ns3", "ep3", "p33"):  true,
		},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{
			makeNSN("te", "ns4", "ep4"): 1,
		},
	}, {
		// Case[15]: change from 0 endpoint address to 1 unnamed port
		previousEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", emptyEndpoint),
		},
		currentEndpoints: []*v1.Endpoints{
			makeTestEndpoints("te", "ns1", "ep1", unnamedPort),
		},
		oldEndpoints: map[ServicePortName][]*BaseEndpointInfo{},
		expectedResult: map[ServicePortName][]*BaseEndpointInfo{
			makeServicePortName("te", "ns1", "ep1", ""): {
				{Endpoint: "1.1.1.1:11", IsLocal: false},
			},
		},
		expectedStaleEndpoints: []ServiceEndpoint{},
		expectedStaleServiceNames: map[ServicePortName]bool{
			makeServicePortName("te", "ns1", "ep1", ""): true,
		},
		expectedHealthchecks: map[types.NamespacednameWithTenantSource]int{},
	},
	}

	for tci, tc := range testCases {
		fp := newFakeProxier()
		fp.hostname = nodeName

		// First check that after adding all previous versions of endpoints,
		// the fp.oldEndpoints is as we expect.
		for i := range tc.previousEndpoints {
			if tc.previousEndpoints[i] != nil {
				fp.addEndpoints(tc.previousEndpoints[i], tpId)
			}
		}
		fp.endpointsMap.Update(fp.endpointsChanges)
		compareEndpointsMaps(t, tci, fp.endpointsMap, tc.oldEndpoints)

		// Now let's call appropriate handlers to get to state we want to be.
		if len(tc.previousEndpoints) != len(tc.currentEndpoints) {
			t.Fatalf("[%d] different lengths of previous and current endpoints", tci)
			continue
		}

		for i := range tc.previousEndpoints {
			prev, curr := tc.previousEndpoints[i], tc.currentEndpoints[i]
			switch {
			case prev == nil:
				fp.addEndpoints(curr, tpId)
			case curr == nil:
				fp.deleteEndpoints(prev, tpId)
			default:
				fp.updateEndpoints(prev, curr, tpId)
			}
		}
		result := fp.endpointsMap.Update(fp.endpointsChanges)
		newMap := fp.endpointsMap
		compareEndpointsMaps(t, tci, newMap, tc.expectedResult)
		if len(result.StaleEndpoints) != len(tc.expectedStaleEndpoints) {
			t.Errorf("[%d] expected %d staleEndpoints, got %d: %v", tci, len(tc.expectedStaleEndpoints), len(result.StaleEndpoints), result.StaleEndpoints)
		}
		for _, x := range tc.expectedStaleEndpoints {
			found := false
			for _, stale := range result.StaleEndpoints {
				if stale == x {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("[%d] expected staleEndpoints[%v], but didn't find it: %v", tci, x, result.StaleEndpoints)
			}
		}
		if len(result.StaleServiceNames) != len(tc.expectedStaleServiceNames) {
			t.Errorf("[%d] expected %d staleServiceNames, got %d: %v", tci, len(tc.expectedStaleServiceNames), len(result.StaleServiceNames), result.StaleServiceNames)
		}
		for svcName := range tc.expectedStaleServiceNames {
			found := false
			for _, stale := range result.StaleServiceNames {
				if stale == svcName {
					found = true
				}
			}
			if !found {
				t.Errorf("[%d] expected staleServiceNames[%v], but didn't find it: %v", tci, svcName, result.StaleServiceNames)
			}
		}
		if !reflect.DeepEqual(result.HCEndpointsLocalIPSize, tc.expectedHealthchecks) {
			t.Errorf("[%d] expected healthchecks %v, got %v", tci, tc.expectedHealthchecks, result.HCEndpointsLocalIPSize)
		}
	}
}

func TestLastChangeTriggerTime(t *testing.T) {
	t0 := time.Date(2018, 01, 01, 0, 0, 0, 0, time.UTC)
	t1 := t0.Add(time.Second)
	t2 := t1.Add(time.Second)
	t3 := t2.Add(time.Second)

	createEndpoints := func(tenant, namespace, name string, triggerTime time.Time) *v1.Endpoints {
		e := makeTestEndpoints(tenant, namespace, name, func(ept *v1.Endpoints) {
			ept.Subsets = []v1.EndpointSubset{{
				Addresses: []v1.EndpointAddress{{IP: "1.1.1.1"}},
				Ports:     []v1.EndpointPort{{Port: 11}},
			}}
		})
		e.Annotations[v1.EndpointsLastChangeTriggerTime] = triggerTime.Format(time.RFC3339Nano)
		return e
	}

	modifyEndpoints := func(endpoints *v1.Endpoints, triggerTime time.Time) *v1.Endpoints {
		e := endpoints.DeepCopy()
		e.Subsets[0].Ports[0].Port++
		e.Annotations[v1.EndpointsLastChangeTriggerTime] = triggerTime.Format(time.RFC3339Nano)
		return e
	}

	sortTimeSlice := func(data []time.Time) {
		sort.Slice(data, func(i, j int) bool { return data[i].Before(data[j]) })
	}

	testCases := []struct {
		name     string
		scenario func(fp *FakeProxier)
		expected []time.Time
	}{
		{
			name: "Single addEndpoints",
			scenario: func(fp *FakeProxier) {
				e := createEndpoints("te", "ns", "ep1", t0)
				fp.addEndpoints(e, tpId)
			},
			expected: []time.Time{t0},
		},
		{
			name: "addEndpoints then updatedEndpoints",
			scenario: func(fp *FakeProxier) {
				e := createEndpoints("te", "ns", "ep1", t0)
				fp.addEndpoints(e, tpId)

				e1 := modifyEndpoints(e, t1)
				fp.updateEndpoints(e, e1, tpId)
			},
			expected: []time.Time{t0, t1},
		},
		{
			name: "Add two endpoints then modify one",
			scenario: func(fp *FakeProxier) {
				e1 := createEndpoints("te", "ns", "ep1", t1)
				fp.addEndpoints(e1, tpId)

				e2 := createEndpoints("te", "ns", "ep2", t2)
				fp.addEndpoints(e2, tpId)

				e11 := modifyEndpoints(e1, t3)
				fp.updateEndpoints(e1, e11, tpId)
			},
			expected: []time.Time{t1, t2, t3},
		},
		{
			name: "Endpoints without annotation set",
			scenario: func(fp *FakeProxier) {
				e := createEndpoints("te", "ns", "ep1", t1)
				delete(e.Annotations, v1.EndpointsLastChangeTriggerTime)
				fp.addEndpoints(e, tpId)
			},
			expected: []time.Time{},
		},
		{
			name: "addEndpoints then deleteEndpoints",
			scenario: func(fp *FakeProxier) {
				e := createEndpoints("te", "ns", "ep1", t1)
				fp.addEndpoints(e, tpId)
				fp.deleteEndpoints(e, tpId)
			},
			expected: []time.Time{},
		},
		{
			name: "add then delete then add again",
			scenario: func(fp *FakeProxier) {
				e := createEndpoints("te", "ns", "ep1", t1)
				fp.addEndpoints(e, tpId)
				fp.deleteEndpoints(e, tpId)
				e = modifyEndpoints(e, t2)
				fp.addEndpoints(e, tpId)
			},
			expected: []time.Time{t2},
		},
	}

	for _, tc := range testCases {
		fp := newFakeProxier()

		tc.scenario(fp)

		result := fp.endpointsMap.Update(fp.endpointsChanges)
		got := result.LastChangeTriggerTimes
		sortTimeSlice(got)
		sortTimeSlice(tc.expected)

		if !reflect.DeepEqual(got, tc.expected) {
			t.Errorf("%s: Invalid LastChangeTriggerTimes, expected: %v, got: %v",
				tc.name, tc.expected, result.LastChangeTriggerTimes)
		}
	}
}

func compareEndpointsMaps(t *testing.T, tci int, newMap EndpointsMap, expected map[ServicePortName][]*BaseEndpointInfo) {
	if len(newMap) != len(expected) {
		t.Errorf("[%d] expected %d results, got %d: %v", tci, len(expected), len(newMap), newMap)
	}
	for x := range expected {
		if len(newMap[x]) != len(expected[x]) {
			t.Errorf("[%d] expected %d endpoints for %v, got %d", tci, len(expected[x]), x, len(newMap[x]))
		} else {
			for i := range expected[x] {
				newEp, ok := newMap[x][i].(*BaseEndpointInfo)
				if !ok {
					t.Errorf("Failed to cast endpointsInfo")
					continue
				}
				if *newEp != *(expected[x][i]) {
					t.Errorf("[%d] expected new[%v][%d] to be %v, got %v", tci, x, i, expected[x][i], newEp)
				}
			}
		}
	}
}
