/*
Copyright 2025.

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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ProviderType is a string enum representing the supported infrastructure providers.
type ProviderType string

const (
	ProviderNutanix   ProviderType = "nutanix"
	ProviderProxmox   ProviderType = "proxmox"
	ProviderAWS       ProviderType = "aws"
	ProviderHarvester ProviderType = "harvester"
	ProviderVSphere   ProviderType = "vsphere"
	ProviderAzure     ProviderType = "azure"
)

// ObjectReference defines a reference to another Kubernetes object.
type ObjectReference struct {
	// APIVersion of the target object.
	// +kubebuilder:validation:MinLength=1
	APIVersion string `json:"apiVersion"`

	// Kind of the target object.
	// +kubebuilder:validation:MinLength=1
	Kind string `json:"kind"`

	// Name of the target object.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ProviderReference contains information about the provider and the backing implementation CR.
type ProviderReference struct {
	// Type of provider backing the cluster.
	// +kubebuilder:validation:Enum=nutanix;aws;harvester;vsphere;azure
	Type ProviderType `json:"type"`

	// Ref to the provider-specific cluster object.
	Ref ObjectReference `json:"ref"`
}

// TenantClusterSpec defines the desired state of TenantCluster.
type TenantClusterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// KubernetesVersion specifies the desired Kubernetes version.
	// +kubebuilder:validation:Pattern=^v\d+\.\d+\.\d+$
	KubernetesVersion string `json:"kubernetesVersion"`

	// Provider contains the reference to the infrastructure provider-specific implementation.
	Provider ProviderReference `json:"provider"`

	// ProviderConfig contains provider-specific configuration in raw JSON.
	// This will be passed through to the specific provider CR.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	ProviderConfig runtime.RawExtension `json:"providerConfig,omitempty"`
}

// TenantClusterPhase represents the lifecycle phase of a TenantCluster.
type TenantClusterPhase string

const (
	PhasePending      TenantClusterPhase = "Pending"
	PhaseProvisioning TenantClusterPhase = "Provisioning"
	PhaseReady        TenantClusterPhase = "Ready"
	PhaseError        TenantClusterPhase = "Error"
	PhaseDeleting     TenantClusterPhase = "Deleting"
)

// TenantClusterStatus defines the observed state of TenantCluster.
type TenantClusterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Phase is a high-level summary of the current state of the cluster.
	// +optional
	Phase TenantClusterPhase `json:"phase,omitempty"`

	// Conditions represents the latest available observations of the cluster’s state.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// TenantCluster is the Schema for the tenantclusters API.
type TenantCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TenantClusterSpec   `json:"spec,omitempty"`
	Status TenantClusterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// TenantClusterList contains a list of TenantCluster.
type TenantClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TenantCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&TenantCluster{}, &TenantClusterList{})
}
