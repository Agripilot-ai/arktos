/*
Copyright 2015 The Kubernetes Authors.
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

package podgc

import (
	"sort"
	"sync"
	"time"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	clientset "k8s.io/client-go/kubernetes"
	corelisters "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/kubernetes/pkg/controller"
	"k8s.io/kubernetes/pkg/util/metrics"

	"k8s.io/klog"
)

const (
	gcCheckPeriod = 20 * time.Second
)

type PodGCController struct {
	kubeClient clientset.Interface

	// all clients to list nodes it cares about, particularly including the current TP client
	kubeClientForNodes map[string]clientset.Interface

	podLister       corelisters.PodLister
	podListerSynced cache.InformerSynced

	deletePod              func(tenant, namespace, name string) error
	terminatedPodThreshold int
}

func NewPodGC(kubeClient clientset.Interface, rpClients map[string]clientset.Interface, podInformer coreinformers.PodInformer, terminatedPodThreshold int) *PodGCController {
	if kubeClient != nil && kubeClient.CoreV1().RESTClient().GetRateLimiter() != nil {
		metrics.RegisterMetricAndTrackRateLimiterUsage("gc_controller", kubeClient.CoreV1().RESTClient().GetRateLimiter())
	}
	gcc := &PodGCController{
		kubeClient:             kubeClient,
		terminatedPodThreshold: terminatedPodThreshold,
		deletePod: func(tenant, namespace, name string) error {
			klog.Infof("PodGC is force deleting Pod: %v/%v/%v", tenant, namespace, name)
			return kubeClient.CoreV1().PodsWithMultiTenancy(namespace, tenant).Delete(name, metav1.NewDeleteOptions(0))
		},
	}

	// key "0" is special case for TP client itself
	// todo: avoid using magic literal "0"
	gcc.kubeClientForNodes = map[string]clientset.Interface{"0": kubeClient}
	for key, value := range rpClients {
		gcc.kubeClientForNodes[key] = value
	}

	gcc.podLister = podInformer.Lister()
	gcc.podListerSynced = podInformer.Informer().HasSynced

	return gcc
}

func (gcc *PodGCController) Run(stop <-chan struct{}) {
	defer utilruntime.HandleCrash()

	klog.Infof("Starting GC controller")
	defer klog.Infof("Shutting down GC controller")

	if !controller.WaitForCacheSync("GC", stop, gcc.podListerSynced) {
		return
	}

	go wait.Until(gcc.gc, gcCheckPeriod, stop)

	<-stop
}

func (gcc *PodGCController) gc() {
	pods, err := gcc.podLister.List(labels.Everything())
	if err != nil {
		klog.Errorf("Error while listing all Pods: %v", err)
		return
	}
	if gcc.terminatedPodThreshold > 0 {
		gcc.gcTerminated(pods)
	}
	gcc.gcOrphaned(pods)
	gcc.gcUnscheduledTerminating(pods)
}

func isPodTerminated(pod *v1.Pod) bool {
	if phase := pod.Status.Phase; phase != v1.PodPending && phase != v1.PodRunning && phase != v1.PodUnknown {
		return true
	}
	return false
}

func (gcc *PodGCController) gcTerminated(pods []*v1.Pod) {
	terminatedPods := []*v1.Pod{}
	for _, pod := range pods {
		if isPodTerminated(pod) {
			terminatedPods = append(terminatedPods, pod)
		}
	}

	terminatedPodCount := len(terminatedPods)
	sort.Sort(byCreationTimestamp(terminatedPods))

	deleteCount := terminatedPodCount - gcc.terminatedPodThreshold

	if deleteCount > terminatedPodCount {
		deleteCount = terminatedPodCount
	}
	if deleteCount > 0 {
		klog.Infof("garbage collecting %v pods", deleteCount)
	}

	var wait sync.WaitGroup
	for i := 0; i < deleteCount; i++ {
		wait.Add(1)
		go func(tenant string, namespace string, name string) {
			defer wait.Done()
			if err := gcc.deletePod(tenant, namespace, name); err != nil {
				// ignore not founds
				defer utilruntime.HandleError(err)
			}
		}(terminatedPods[i].Tenant, terminatedPods[i].Namespace, terminatedPods[i].Name)
	}
	wait.Wait()
}

// gcOrphaned deletes pods that are bound to nodes that don't exist.
func (gcc *PodGCController) gcOrphaned(pods []*v1.Pod) {
	klog.V(4).Infof("GC'ing orphaned")
	// We want to get list of Nodes from the etcd, to make sure that it's as fresh as possible.

	// get nodes from resource provider clients
	allRpNodes, errs := getLatestNodes(gcc.kubeClientForNodes)

	// check errors and aggregate nodes
	if len(errs) == len(gcc.kubeClientForNodes) {
		// avoid garbage collection when all kubeclients are not accessible
		klog.Errorf("Error listing nodes from all resource partition. err: %v", errs)
		return
	}

	nodeNames := sets.NewString()
	for _, nodes := range allRpNodes {
		for _, node := range nodes.Items {
			nodeNames.Insert(node.Name)
		}
	}

	for _, pod := range pods {
		if pod.Spec.NodeName == "" {
			continue
		}
		if nodeNames.Has(pod.Spec.NodeName) {
			continue
		}
		klog.V(2).Infof("Found orphaned Pod %v/%v/%v assigned to the Node %v. Deleting.", pod.Tenant, pod.Namespace, pod.Name, pod.Spec.NodeName)
		if err := gcc.deletePod(pod.Tenant, pod.Namespace, pod.Name); err != nil {
			utilruntime.HandleError(err)
		} else {
			klog.V(0).Infof("Forced deletion of orphaned Pod %v/%v/%v succeeded", pod.Tenant, pod.Namespace, pod.Name)
		}
	}
}

func getLatestNodes(kubeClients map[string]clientset.Interface) (map[string]*v1.NodeList, map[string]error) {
	allRpNodes := make(map[string]*v1.NodeList, len(kubeClients))
	errs := make(map[string]error, len(kubeClients))
	var wg sync.WaitGroup
	wg.Add(len(kubeClients))
	var lock sync.Mutex
	for rpId, client := range kubeClients {
		go func(resourceProviderId string, rpClient clientset.Interface, nodeLists map[string]*v1.NodeList, errs map[string]error, writeLock *sync.Mutex) {
			defer wg.Done()
			nodes, err := rpClient.CoreV1().Nodes().List(metav1.ListOptions{})
			if err != nil {
				writeLock.Lock()
				errs[resourceProviderId] = err
				klog.Errorf("Error listing nodes. err: %v", errs)
				writeLock.Unlock()
				return
			}
			writeLock.Lock()
			nodeLists[resourceProviderId] = nodes
			writeLock.Unlock()
		}(rpId, client, allRpNodes, errs, &lock)
	}
	wg.Wait()
	return allRpNodes, errs
}

// gcUnscheduledTerminating deletes pods that are terminating and haven't been scheduled to a particular node.
func (gcc *PodGCController) gcUnscheduledTerminating(pods []*v1.Pod) {
	klog.V(4).Infof("GC'ing unscheduled pods which are terminating.")

	for _, pod := range pods {
		if pod.DeletionTimestamp == nil || len(pod.Spec.NodeName) > 0 {
			continue
		}

		klog.V(2).Infof("Found unscheduled terminating Pod %v/%v/%v not assigned to any Node. Deleting.", pod.Tenant, pod.Namespace, pod.Name)
		if err := gcc.deletePod(pod.Tenant, pod.Namespace, pod.Name); err != nil {
			utilruntime.HandleError(err)
		} else {
			klog.V(0).Infof("Forced deletion of unscheduled terminating Pod %v/%v/%v succeeded", pod.Tenant, pod.Namespace, pod.Name)
		}
	}
}

// byCreationTimestamp sorts a list by creation timestamp, using their names as a tie breaker.
type byCreationTimestamp []*v1.Pod

func (o byCreationTimestamp) Len() int      { return len(o) }
func (o byCreationTimestamp) Swap(i, j int) { o[i], o[j] = o[j], o[i] }

func (o byCreationTimestamp) Less(i, j int) bool {
	if o[i].CreationTimestamp.Equal(&o[j].CreationTimestamp) {
		return o[i].Name < o[j].Name
	}
	return o[i].CreationTimestamp.Before(&o[j].CreationTimestamp)
}