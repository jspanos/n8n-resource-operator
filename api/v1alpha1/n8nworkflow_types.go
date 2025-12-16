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

// SyncPolicy defines how the operator syncs workflows with n8n
// +kubebuilder:validation:Enum=Always;CreateOnly;Manual
type SyncPolicy string

const (
	// SyncPolicyAlways continuously syncs the workflow to n8n (default)
	// UI changes will be overwritten on each reconciliation
	SyncPolicyAlways SyncPolicy = "Always"

	// SyncPolicyCreateOnly only creates the workflow, never updates
	// Allows manual editing in the n8n UI without being overwritten
	SyncPolicyCreateOnly SyncPolicy = "CreateOnly"

	// SyncPolicyManual pauses all sync operations
	// Useful during active development in the UI
	SyncPolicyManual SyncPolicy = "Manual"
)

// WorkflowSpec defines the n8n workflow specification
type WorkflowSpec struct {
	// Name of the workflow (must be unique in n8n)
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Nodes in the workflow
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Nodes []runtime.RawExtension `json:"nodes,omitempty"`

	// Connections between nodes
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Connections *runtime.RawExtension `json:"connections,omitempty"`

	// Workflow settings
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Settings *runtime.RawExtension `json:"settings,omitempty"`

	// Static data for the workflow
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	StaticData *runtime.RawExtension `json:"staticData,omitempty"`

	// Pinned data for nodes
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	PinData *runtime.RawExtension `json:"pinData,omitempty"`
}

// N8nWorkflowSpec defines the desired state of N8nWorkflow
type N8nWorkflowSpec struct {
	// InstanceRef references an N8nInstance by name
	// The N8nInstance must exist in the operator namespace
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	InstanceRef string `json:"instanceRef"`

	// SyncPolicy defines how the operator handles synchronization with n8n
	// - Always: Continuously sync, overwriting UI changes (default)
	// - CreateOnly: Create workflow but never update, allowing UI edits
	// - Manual: Pause all sync operations
	// +kubebuilder:default=Always
	// +optional
	SyncPolicy SyncPolicy `json:"syncPolicy,omitempty"`

	// Whether the workflow should be active
	// +kubebuilder:default=true
	// +optional
	Active bool `json:"active,omitempty"`

	// The n8n workflow definition
	// +kubebuilder:validation:Required
	Workflow WorkflowSpec `json:"workflow"`
}

// N8nWorkflowStatus defines the observed state of N8nWorkflow
type N8nWorkflowStatus struct {
	// The n8n internal workflow ID
	// +optional
	WorkflowID string `json:"workflowId,omitempty"`

	// Whether the workflow is currently active in n8n
	// +optional
	Active bool `json:"active,omitempty"`

	// Last time the workflow was synced to n8n
	// +optional
	LastSyncTime *metav1.Time `json:"lastSyncTime,omitempty"`

	// The webhook URL if the workflow has a webhook trigger
	// +optional
	WebhookURL string `json:"webhookUrl,omitempty"`

	// The generation observed by the controller
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Hash of the workflow spec used for drift detection
	// Only updates when spec actually changes
	// +optional
	SpecHash string `json:"specHash,omitempty"`

	// Conditions of the workflow
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// Condition types for N8nWorkflow
const (
	// ConditionTypeReady indicates the workflow is ready and synced
	ConditionTypeReady = "Ready"

	// ConditionTypeSynced indicates the workflow has been synced to n8n
	ConditionTypeSynced = "Synced"
)

// Condition reasons
const (
	ReasonSyncSucceeded   = "SyncSucceeded"
	ReasonSyncFailed      = "SyncFailed"
	ReasonActivated       = "Activated"
	ReasonDeactivated     = "Deactivated"
	ReasonActivationError = "ActivationError"
	ReasonAPIError        = "APIError"
	ReasonDeleting        = "Deleting"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=n8nwf;wf
// +kubebuilder:printcolumn:name="Instance",type=string,JSONPath=`.spec.instanceRef`
// +kubebuilder:printcolumn:name="Workflow Name",type=string,JSONPath=`.spec.workflow.name`
// +kubebuilder:printcolumn:name="Active",type=boolean,JSONPath=`.status.active`
// +kubebuilder:printcolumn:name="Sync Policy",type=string,JSONPath=`.spec.syncPolicy`
// +kubebuilder:printcolumn:name="Workflow ID",type=string,JSONPath=`.status.workflowId`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// N8nWorkflow is the Schema for the n8nworkflows API
type N8nWorkflow struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +kubebuilder:validation:Required
	Spec   N8nWorkflowSpec   `json:"spec"`
	Status N8nWorkflowStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// N8nWorkflowList contains a list of N8nWorkflow
type N8nWorkflowList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []N8nWorkflow `json:"items"`
}

func init() {
	SchemeBuilder.Register(&N8nWorkflow{}, &N8nWorkflowList{})
}
