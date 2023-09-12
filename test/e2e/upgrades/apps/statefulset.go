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

package upgrades

import (
	"github.com/onsi/ginkgo"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/version"

	"k8s.io/kubernetes/test/e2e/framework"
	"k8s.io/kubernetes/test/e2e/upgrades"
)

// StatefulSetUpgradeTest implements an upgrade test harness for StatefulSet upgrade testing.
type StatefulSetUpgradeTest struct {
	tester  *framework.StatefulSetTester
	service *v1.Service
	set     *appsv1.StatefulSet
}

// Name returns the tracking name of the test.
func (StatefulSetUpgradeTest) Name() string { return "[sig-apps] statefulset-upgrade" }

// Skip returns true when this test can be skipped.
func (StatefulSetUpgradeTest) Skip(upgCtx upgrades.UpgradeContext) bool {
	minVersion := version.MustParseSemantic("1.5.0")

	for _, vCtx := range upgCtx.Versions {
		if vCtx.Version.LessThan(minVersion) {
			return true
		}
	}
	return false
}

// Setup creates a StatefulSet and a HeadlessService. It verifies the basic SatefulSet properties
func (t *StatefulSetUpgradeTest) Setup(f *framework.Framework) {
	ssName := "ss"
	labels := map[string]string{
		"foo": "bar",
		"baz": "blah",
	}
	headlessSvcName := "test"
	statefulPodMounts := []v1.VolumeMount{{Name: "datadir", MountPath: "/data/"}}
	podMounts := []v1.VolumeMount{{Name: "home", MountPath: "/home"}}
	ns := f.Namespace.Name
	t.set = framework.NewStatefulSet(ssName, ns, headlessSvcName, 2, statefulPodMounts, podMounts, labels)
	t.service = framework.CreateStatefulSetService(ssName, labels)
	*(t.set.Spec.Replicas) = 3
	t.tester = framework.NewStatefulSetTester(f.ClientSet)
	t.tester.PauseNewPods(t.set)

	ginkgo.By("Creating service " + headlessSvcName + " in namespace " + ns)
	_, err := f.ClientSet.CoreV1().Services(ns).Create(t.service)
	framework.ExpectNoError(err)

	ginkgo.By("Creating statefulset " + ssName + " in namespace " + ns)
	*(t.set.Spec.Replicas) = 3
	_, err = f.ClientSet.AppsV1().StatefulSets(ns).Create(t.set)
	framework.ExpectNoError(err)

	ginkgo.By("Saturating stateful set " + t.set.Name)
	t.tester.Saturate(t.set)
	t.verify()
	t.restart()
	t.verify()
}

// Test waits for the upgrade to complete and verifies the StatefulSet basic functionality
func (t *StatefulSetUpgradeTest) Test(f *framework.Framework, done <-chan struct{}, upgrade upgrades.UpgradeType) {
	<-done
	t.verify()
}

// Teardown deletes all StatefulSets
func (t *StatefulSetUpgradeTest) Teardown(f *framework.Framework) {
	framework.DeleteAllStatefulSets(f.ClientSet, t.set.Name)
}

func (t *StatefulSetUpgradeTest) verify() {
	ginkgo.By("Verifying statefulset mounted data directory is usable")
	framework.ExpectNoError(t.tester.CheckMount(t.set, "/data"))

	ginkgo.By("Verifying statefulset provides a stable hostname for each pod")
	framework.ExpectNoError(t.tester.CheckHostname(t.set))

	ginkgo.By("Verifying statefulset set proper service name")
	framework.ExpectNoError(t.tester.CheckServiceName(t.set, t.set.Spec.ServiceName))

	cmd := "echo $(hostname) > /data/hostname; sync;"
	ginkgo.By("Running " + cmd + " in all stateful pods")
	framework.ExpectNoError(t.tester.ExecInStatefulPods(t.set, cmd))
}

func (t *StatefulSetUpgradeTest) restart() {
	ginkgo.By("Restarting statefulset " + t.set.Name)
	t.tester.Restart(t.set)
	t.tester.WaitForRunningAndReady(*t.set.Spec.Replicas, t.set)
}