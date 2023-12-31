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

package constants

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/pkg/errors"

	"k8s.io/apimachinery/pkg/util/version"
	kubeadmapi "k8s.io/kubernetes/cmd/kubeadm/app/apis/kubeadm"
)

func TestGetStaticPodDirectory(t *testing.T) {
	expected := "/etc/kubernetes/manifests"
	actual := GetStaticPodDirectory()

	if actual != expected {
		t.Errorf(
			"failed GetStaticPodDirectory:\n\texpected: %s\n\t  actual: %s",
			expected,
			actual,
		)
	}
}

func TestGetAdminKubeConfigPath(t *testing.T) {
	expected := filepath.Join(KubernetesDir, AdminKubeConfigFileName)
	actual := GetAdminKubeConfigPath()

	if actual != expected {
		t.Errorf(
			"failed GetAdminKubeConfigPath:\n\texpected: %s\n\t  actual: %s",
			expected,
			actual,
		)
	}
}

func TestGetBootstrapKubeletKubeConfigPath(t *testing.T) {
	expected := "/etc/kubernetes/bootstrap-kubelet.conf"
	actual := GetBootstrapKubeletKubeConfigPath()

	if actual != expected {
		t.Errorf(
			"failed GetBootstrapKubeletKubeConfigPath:\n\texpected: %s\n\t  actual: %s",
			expected,
			actual,
		)
	}
}

func TestGetKubeletKubeConfigPath(t *testing.T) {
	expected := "/etc/kubernetes/kubelet.conf"
	actual := GetKubeletKubeConfigPath()

	if actual != expected {
		t.Errorf(
			"failed GetKubeletKubeConfigPath:\n\texpected: %s\n\t  actual: %s",
			expected,
			actual,
		)
	}
}

func TestGetStaticPodFilepath(t *testing.T) {
	var tests = []struct {
		componentName, manifestsDir, expected string
	}{
		{
			componentName: "kube-apiserver",
			manifestsDir:  "/etc/kubernetes/manifests",
			expected:      "/etc/kubernetes/manifests/kube-apiserver.yaml",
		},
		{
			componentName: "kube-controller-manager",
			manifestsDir:  "/etc/kubernetes/manifests/",
			expected:      "/etc/kubernetes/manifests/kube-controller-manager.yaml",
		},
		{
			componentName: "foo",
			manifestsDir:  "/etc/bar/",
			expected:      "/etc/bar/foo.yaml",
		},
	}
	for _, rt := range tests {
		t.Run(rt.componentName, func(t *testing.T) {
			actual := GetStaticPodFilepath(rt.componentName, rt.manifestsDir)
			if actual != rt.expected {
				t.Errorf(
					"failed GetStaticPodFilepath:\n\texpected: %s\n\t  actual: %s",
					rt.expected,
					actual,
				)
			}
		})
	}
}

func TestAddSelfHostedPrefix(t *testing.T) {
	var tests = []struct {
		componentName, expected string
	}{
		{
			componentName: "kube-apiserver",
			expected:      "self-hosted-kube-apiserver",
		},
		{
			componentName: "kube-controller-manager",
			expected:      "self-hosted-kube-controller-manager",
		},
		{
			componentName: "kube-scheduler",
			expected:      "self-hosted-kube-scheduler",
		},
		{
			componentName: "foo",
			expected:      "self-hosted-foo",
		},
	}
	for _, rt := range tests {
		t.Run(rt.componentName, func(t *testing.T) {
			actual := AddSelfHostedPrefix(rt.componentName)
			if actual != rt.expected {
				t.Errorf(
					"failed AddSelfHostedPrefix:\n\texpected: %s\n\t  actual: %s",
					rt.expected,
					actual,
				)
			}
		})
	}
}

func TestEtcdSupportedVersion(t *testing.T) {
	var tests = []struct {
		kubernetesVersion string
		expectedVersion   *version.Version
		expectedError     error
	}{
		{
			kubernetesVersion: "1.x.0",
			expectedVersion:   nil,
			expectedError:     errors.New("illegal version string \"1.x.0\""),
		},
		{
			kubernetesVersion: "1.14.0",
			expectedVersion:   version.MustParseSemantic("3.4.3"),
			expectedError:     nil,
		},
		{
			kubernetesVersion: "1.14.1",
			expectedVersion:   version.MustParseSemantic("3.4.3"),
			expectedError:     nil,
		},
		{
			kubernetesVersion: "1.16.0",
			expectedVersion:   nil,
			expectedError:     errors.New("Unsupported or unknown Kubernetes version(1.16.0)"),
		},
		{
			kubernetesVersion: MinimumControlPlaneVersion.WithPatch(1).String(),
			expectedVersion:   version.MustParseSemantic(SupportedEtcdVersion[uint8(MinimumControlPlaneVersion.Minor())]),
			expectedError:     nil,
		},
		{
			kubernetesVersion: CurrentKubernetesVersion.String(),
			expectedVersion:   version.MustParseSemantic(SupportedEtcdVersion[uint8(CurrentKubernetesVersion.Minor())]),
			expectedError:     nil,
		},
	}
	for _, rt := range tests {
		t.Run(rt.kubernetesVersion, func(t *testing.T) {
			actualVersion, actualError := EtcdSupportedVersion(rt.kubernetesVersion)
			if actualError != nil {
				if rt.expectedError == nil {
					t.Errorf("failed EtcdSupportedVersion:\n\texpected no error, but got: %v", actualError)
				} else if actualError.Error() != rt.expectedError.Error() {
					t.Errorf(
						"failed EtcdSupportedVersion:\n\texpected error: %v\n\t  actual error: %v",
						rt.expectedError,
						actualError,
					)
				}
			} else {
				if rt.expectedError != nil {
					t.Errorf("failed EtcdSupportedVersion:\n\texpected error: %v, but got no error", rt.expectedError)
				} else if strings.Compare(actualVersion.String(), rt.expectedVersion.String()) != 0 {
					t.Errorf(
						"failed EtcdSupportedVersion:\n\texpected version: %s\n\t  actual version: %s",
						rt.expectedVersion.String(),
						actualVersion.String(),
					)
				}
			}
		})
	}
}

func TestGetKubeDNSVersion(t *testing.T) {
	var tests = []struct {
		dns      kubeadmapi.DNSAddOnType
		expected string
	}{
		{
			dns:      kubeadmapi.KubeDNS,
			expected: KubeDNSVersion,
		},
		{
			dns:      kubeadmapi.CoreDNS,
			expected: CoreDNSVersion,
		},
	}
	for _, rt := range tests {
		t.Run(string(rt.dns), func(t *testing.T) {
			actualDNSVersion := GetDNSVersion(rt.dns)
			if actualDNSVersion != rt.expected {
				t.Errorf(
					"failed GetDNSVersion:\n\texpected: %s\n\t  actual: %s",
					rt.expected,
					actualDNSVersion,
				)
			}
		})
	}
}
