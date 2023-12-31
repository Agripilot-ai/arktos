// +build !ignore_autogenerated

/*
Copyright The Kubernetes Authors.
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

// Code generated by deepcopy-gen. DO NOT EDIT.

package config

import (
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *CloudProviderConfiguration) DeepCopyInto(out *CloudProviderConfiguration) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new CloudProviderConfiguration.
func (in *CloudProviderConfiguration) DeepCopy() *CloudProviderConfiguration {
	if in == nil {
		return nil
	}
	out := new(CloudProviderConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *DeprecatedControllerConfiguration) DeepCopyInto(out *DeprecatedControllerConfiguration) {
	*out = *in
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new DeprecatedControllerConfiguration.
func (in *DeprecatedControllerConfiguration) DeepCopy() *DeprecatedControllerConfiguration {
	if in == nil {
		return nil
	}
	out := new(DeprecatedControllerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GenericControllerManagerConfiguration) DeepCopyInto(out *GenericControllerManagerConfiguration) {
	*out = *in
	out.MinResyncPeriod = in.MinResyncPeriod
	out.ClientConnection = in.ClientConnection
	out.ControllerStartInterval = in.ControllerStartInterval
	out.LeaderElection = in.LeaderElection
	if in.Controllers != nil {
		in, out := &in.Controllers, &out.Controllers
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	out.Debugging = in.Debugging
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GenericControllerManagerConfiguration.
func (in *GenericControllerManagerConfiguration) DeepCopy() *GenericControllerManagerConfiguration {
	if in == nil {
		return nil
	}
	out := new(GenericControllerManagerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KubeCloudSharedConfiguration) DeepCopyInto(out *KubeCloudSharedConfiguration) {
	*out = *in
	out.CloudProvider = in.CloudProvider
	out.RouteReconciliationPeriod = in.RouteReconciliationPeriod
	out.NodeMonitorPeriod = in.NodeMonitorPeriod
	out.NodeSyncPeriod = in.NodeSyncPeriod
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KubeCloudSharedConfiguration.
func (in *KubeCloudSharedConfiguration) DeepCopy() *KubeCloudSharedConfiguration {
	if in == nil {
		return nil
	}
	out := new(KubeCloudSharedConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *KubeControllerManagerConfiguration) DeepCopyInto(out *KubeControllerManagerConfiguration) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.Generic.DeepCopyInto(&out.Generic)
	out.KubeCloudShared = in.KubeCloudShared
	out.AttachDetachController = in.AttachDetachController
	out.CSRSigningController = in.CSRSigningController
	out.DaemonSetController = in.DaemonSetController
	out.DeploymentController = in.DeploymentController
	out.DeprecatedController = in.DeprecatedController
	out.EndpointController = in.EndpointController
	in.GarbageCollectorController.DeepCopyInto(&out.GarbageCollectorController)
	out.HPAController = in.HPAController
	out.JobController = in.JobController
	out.NamespaceController = in.NamespaceController
	out.NodeIPAMController = in.NodeIPAMController
	out.NodeLifecycleController = in.NodeLifecycleController
	out.PersistentVolumeBinderController = in.PersistentVolumeBinderController
	out.PodGCController = in.PodGCController
	out.ReplicaSetController = in.ReplicaSetController
	out.ReplicationController = in.ReplicationController
	out.ResourceQuotaController = in.ResourceQuotaController
	out.SAController = in.SAController
	out.ServiceController = in.ServiceController
	out.TenantController = in.TenantController
	out.TTLAfterFinishedController = in.TTLAfterFinishedController
	out.MizarArktosNetworkController = in.MizarArktosNetworkController
	return
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new KubeControllerManagerConfiguration.
func (in *KubeControllerManagerConfiguration) DeepCopy() *KubeControllerManagerConfiguration {
	if in == nil {
		return nil
	}
	out := new(KubeControllerManagerConfiguration)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *KubeControllerManagerConfiguration) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
