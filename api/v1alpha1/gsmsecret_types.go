/*
Copyright 2025 Zera Holladay.

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

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// Annotation keys for configuration overrides.
const (
	AnnotationKSA         = "secrets.gsm-operator.io/ksa"
	AnnotationGSA         = "secrets.gsm-operator.io/gsa"
	AnnotationWIFAudience = "secrets.gsm-operator.io/wif-audience"
	AnnotationRelease     = "secrets.gsm-operator.io/release"
)

// GSMSecretSpec defines the desired state of GSMSecret.
type GSMSecretSpec struct {
	// TargetSecret describes the Kubernetes Secret to create or update.
	// +kubebuilder:validation:Required
	TargetSecret GSMSecretTargetSecret `json:"targetSecret"`

	// Secrets is the list of GSM secrets to materialize into the target Secret.
	// +kubebuilder:validation:MinItems=1
	Secrets []GSMSecretEntry `json:"gsmSecrets"`
}

// GSMSecretTargetSecret describes the Kubernetes Secret to materialize into.
type GSMSecretTargetSecret struct {
	// Name is the name of the Kubernetes Secret to create or update.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
	Name string `json:"name"`
}

// GSMSecretEntry describes a single GSM secret to materialize.
type GSMSecretEntry struct {
	// Key is the key under which the value will be stored in the target Secret's data.
	// Example: "MY_ENVVAR".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z0-9._-]+$`
	Key string `json:"key"`

	// ProjectID is the GCP project that owns the Secret Manager secret.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[a-z][a-z0-9-]{4,28}[a-z0-9]$`
	ProjectID string `json:"projectId"`

	// SecretID is the name of the Secret Manager secret.
	// Example: "my-secret".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^[A-Za-z][A-Za-z0-9_-]{0,253}[A-Za-z0-9]$`
	SecretID string `json:"secretId"`

	// Version is the Secret Manager secret version to materialize.
	// Examples: "7" or "latest".
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:Pattern=`^(latest|[1-9][0-9]*)$`
	Version string `json:"version"`
}

// GSMSecretStatus defines the observed state of GSMSecret.
type GSMSecretStatus struct {
	// ObservedGeneration is the most recent generation observed by the controller.
	// It is used to determine whether the status reflects the current desired state.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// Conditions represent the current state of the GSMSecret resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Ready": the Secret has been successfully materialized.
	// - "Progressing": the Secret is being created or updated.
	// - "Degraded": the controller failed to reach or maintain the desired state.
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// GSMSecret is the Schema for the gsmsecrets API.
type GSMSecret struct {
	metav1.TypeMeta `json:",inline"`

	// Metadata is standard object metadata.
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of GSMSecret.
	// +required
	Spec GSMSecretSpec `json:"spec"`

	// Status defines the observed state of GSMSecret.
	// +optional
	Status GSMSecretStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GSMSecretList contains a list of GSMSecret.
type GSMSecretList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []GSMSecret `json:"items"`
}

func init() {
	SchemeBuilder.Register(&GSMSecret{}, &GSMSecretList{})
}
