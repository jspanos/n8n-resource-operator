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
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ServiceRef references a Kubernetes service for n8n
type ServiceRef struct {
	// Name of the n8n service
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace of the n8n service
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// Port of the n8n service
	// +kubebuilder:default=5678
	// +optional
	Port int `json:"port,omitempty"`
}

// CredentialsRef references the credentials for n8n API authentication
type CredentialsRef struct {
	// SecretName is the name of the secret containing the API key
	// The secret must be in the same namespace as the N8nInstance (operator namespace)
	// +kubebuilder:validation:Required
	SecretName string `json:"secretName"`

	// SecretKey is the key in the secret containing the API key
	// +kubebuilder:default=api-key
	// +optional
	SecretKey string `json:"secretKey,omitempty"`
}

// N8nInstanceSpec defines the desired state of N8nInstance
type N8nInstanceSpec struct {
	// URL is the full base URL of the n8n instance API
	// Use this for cloud-hosted n8n (e.g., "https://myorg.app.n8n.cloud")
	// or any externally accessible n8n instance
	// Either URL or ServiceRef must be specified, but not both
	// +optional
	URL string `json:"url,omitempty"`

	// ServiceRef references a Kubernetes service running n8n
	// Use this for self-hosted n8n within the same Kubernetes cluster
	// Either URL or ServiceRef must be specified, but not both
	// +optional
	ServiceRef *ServiceRef `json:"serviceRef,omitempty"`

	// Credentials references the secret containing the n8n API key
	// The secret must be in the same namespace as this N8nInstance
	// +kubebuilder:validation:Required
	Credentials CredentialsRef `json:"credentials"`
}

// N8nInstanceStatus defines the observed state of N8nInstance
type N8nInstanceStatus struct {
	// Ready indicates whether the n8n instance is reachable and authenticated
	// +optional
	Ready bool `json:"ready,omitempty"`

	// URL is the resolved URL used to connect to the n8n instance
	// +optional
	URL string `json:"url,omitempty"`

	// LastHealthCheck is the last time the instance was successfully health-checked
	// +optional
	LastHealthCheck *metav1.Time `json:"lastHealthCheck,omitempty"`

	// The generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Conditions of the n8n instance
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Condition types for N8nInstance
const (
	// InstanceConditionTypeReady indicates the instance is ready and connected
	InstanceConditionTypeReady = "Ready"
)

// Condition reasons for N8nInstance
const (
	InstanceReasonConnected       = "Connected"
	InstanceReasonConnectionError = "ConnectionError"
	InstanceReasonAuthError       = "AuthenticationError"
	InstanceReasonInvalidConfig   = "InvalidConfiguration"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=n8ni;instance
// +kubebuilder:printcolumn:name="URL",type=string,JSONPath=`.status.url`
// +kubebuilder:printcolumn:name="Ready",type=boolean,JSONPath=`.status.ready`
// +kubebuilder:printcolumn:name="Last Check",type=date,JSONPath=`.status.lastHealthCheck`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// N8nInstance is the Schema for the n8ninstances API
// It defines a connection to an n8n instance (cloud or self-hosted)
type N8nInstance struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   N8nInstanceSpec   `json:"spec"`
	Status N8nInstanceStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// N8nInstanceList contains a list of N8nInstance
type N8nInstanceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []N8nInstance `json:"items"`
}

// GetResolvedURL returns the URL to use for connecting to this n8n instance
func (i *N8nInstance) GetResolvedURL() string {
	if i.Spec.URL != "" {
		return i.Spec.URL
	}
	if i.Spec.ServiceRef != nil {
		port := i.Spec.ServiceRef.Port
		if port == 0 {
			port = 5678
		}
		return fmt.Sprintf("http://%s.%s.svc.cluster.local:%d",
			i.Spec.ServiceRef.Name,
			i.Spec.ServiceRef.Namespace,
			port)
	}
	return ""
}

// GetSecretKey returns the key to use when reading the API key from the secret
func (i *N8nInstance) GetSecretKey() string {
	if i.Spec.Credentials.SecretKey != "" {
		return i.Spec.Credentials.SecretKey
	}
	return "api-key"
}

func init() {
	SchemeBuilder.Register(&N8nInstance{}, &N8nInstanceList{})
}
