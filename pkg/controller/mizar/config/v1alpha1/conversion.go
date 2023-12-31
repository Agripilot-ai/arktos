/*
Copyright 2022 Authors of Arktos.

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

package v1alpha1

import (
	"k8s.io/apimachinery/pkg/conversion"
	"k8s.io/kube-controller-manager/config/v1alpha1"
	"k8s.io/kubernetes/pkg/controller/mizar/config"
)

// Important! The public back-and-forth conversion functions for the types in this package
// with JobControllerConfiguration types need to be manually exposed like this in order for
// other packages that reference this package to be able to call these conversion functions
// in an autogenerated manner.
// TODO: Fix the bug in conversion-gen so it automatically discovers these Convert_* functions
// in autogenerated code as well.

// Convert_v1alpha1_MizarArktosNetworkControllerConfiguration_To_config_MizarArktosNetworkControllerConfiguration is an autogenerated conversion function.
func Convert_v1alpha1_MizarArktosNetworkControllerConfiguration_To_config_MizarArktosNetworkControllerConfiguration(
	in *v1alpha1.MizarArktosNetworkControllerConfiguration, out *config.MizarArktosNetworkControllerConfiguration, s conversion.Scope) error {
	return autoConvert_v1alpha1_MizarArktosNetworkControllerConfiguration_To_config_MizarArktosNetworkControllerConfiguration(in, out, s)
}

// Convert_config_JobControllerConfiguration_To_v1alpha1_JobControllerConfiguration is an autogenerated conversion function.
func Convert_config_MizarArktosNetworkControllerConfiguration_To_v1alpha1_MizarArktosNetworkControllerConfiguration(
	in *config.MizarArktosNetworkControllerConfiguration, out *v1alpha1.MizarArktosNetworkControllerConfiguration, s conversion.Scope) error {
	return autoConvert_config_MizarArktosNetworkControllerConfiguration_To_v1alpha1_MizarArktosNetworkControllerConfiguration(in, out, s)
}
