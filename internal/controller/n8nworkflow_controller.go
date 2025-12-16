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

package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	n8nv1alpha1 "github.com/jspanos/n8n-resource-operator/api/v1alpha1"
	"github.com/jspanos/n8n-resource-operator/internal/n8n"
)

const (
	// finalizerName is the finalizer used to clean up workflows in n8n
	finalizerName = "n8n.slys.dev/workflow-cleanup"

	// Default requeue interval for periodic reconciliation
	defaultRequeueInterval = 5 * time.Minute

	// Error requeue interval
	errorRequeueInterval = 30 * time.Second
)

// N8nWorkflowReconciler reconciles a N8nWorkflow object
type N8nWorkflowReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder

	// Default n8n configuration (can be overridden per-workflow)
	DefaultN8nURL    string
	DefaultN8nAPIKey string
}

// +kubebuilder:rbac:groups=n8n.slys.dev,resources=n8nworkflows,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=n8n.slys.dev,resources=n8nworkflows/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=n8n.slys.dev,resources=n8nworkflows/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *N8nWorkflowReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling N8nWorkflow")

	// Fetch the N8nWorkflow instance
	workflow := &n8nv1alpha1.N8nWorkflow{}
	if err := r.Get(ctx, req.NamespacedName, workflow); err != nil {
		if errors.IsNotFound(err) {
			log.Info("N8nWorkflow resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get N8nWorkflow")
		return ctrl.Result{}, err
	}

	// Get n8n API client
	n8nClient, err := r.getN8nClient(ctx, workflow)
	if err != nil {
		log.Error(err, "Failed to create n8n client")
		r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
			n8nv1alpha1.ReasonAPIError, fmt.Sprintf("Failed to create n8n client: %v", err))
		if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
	}

	// Handle deletion
	if !workflow.DeletionTimestamp.IsZero() {
		return r.handleDeletion(ctx, workflow, n8nClient)
	}

	// Add finalizer if it doesn't exist
	if !controllerutil.ContainsFinalizer(workflow, finalizerName) {
		controllerutil.AddFinalizer(workflow, finalizerName)
		if err := r.Update(ctx, workflow); err != nil {
			log.Error(err, "Failed to add finalizer")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile the workflow
	return r.reconcileWorkflow(ctx, workflow, n8nClient)
}

// getN8nClient creates an n8n API client for the workflow
func (r *N8nWorkflowReconciler) getN8nClient(ctx context.Context, workflow *n8nv1alpha1.N8nWorkflow) (*n8n.Client, error) {
	var baseURL, apiKey string

	// Determine n8n URL
	if workflow.Spec.N8nRef != nil {
		// Use explicit URL if specified (takes precedence)
		if workflow.Spec.N8nRef.URL != "" {
			baseURL = workflow.Spec.N8nRef.URL
		} else {
			// Construct URL from service reference
			namespace := workflow.Spec.N8nRef.Namespace
			if namespace == "" {
				namespace = workflow.Namespace
			}
			name := workflow.Spec.N8nRef.Name
			if name == "" {
				name = "n8n-service"
			}
			port := workflow.Spec.N8nRef.Port
			if port == 0 {
				port = 5678
			}
			baseURL = fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", name, namespace, port)
		}

		// Get API key from secret if specified
		if workflow.Spec.N8nRef.SecretRef != nil {
			secretNamespace := workflow.Spec.N8nRef.Namespace
			if secretNamespace == "" {
				secretNamespace = workflow.Namespace
			}
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{
				Name:      workflow.Spec.N8nRef.SecretRef.Name,
				Namespace: secretNamespace,
			}
			if err := r.Get(ctx, secretKey, secret); err != nil {
				return nil, fmt.Errorf("failed to get API key secret: %w", err)
			}
			key := workflow.Spec.N8nRef.SecretRef.Key
			if key == "" {
				key = "api-key"
			}
			apiKeyBytes, ok := secret.Data[key]
			if !ok {
				return nil, fmt.Errorf("secret %s does not contain key %s", secretKey, key)
			}
			apiKey = string(apiKeyBytes)
		}
	}

	// Fall back to defaults from environment/config
	if baseURL == "" {
		baseURL = r.DefaultN8nURL
		if baseURL == "" {
			baseURL = os.Getenv("N8N_API_URL")
		}
	}
	if apiKey == "" {
		apiKey = r.DefaultN8nAPIKey
		if apiKey == "" {
			apiKey = os.Getenv("N8N_API_KEY")
		}
	}

	// Validate required configuration
	if baseURL == "" {
		return nil, fmt.Errorf("no n8n URL configured: specify n8nRef.url, n8nRef.name/namespace/port, or set N8N_API_URL environment variable")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no n8n API key configured: specify n8nRef.secretRef or set N8N_API_KEY environment variable")
	}

	return n8n.NewClient(baseURL, apiKey), nil
}

// reconcileWorkflow syncs the workflow to n8n
func (r *N8nWorkflowReconciler) reconcileWorkflow(ctx context.Context, workflow *n8nv1alpha1.N8nWorkflow, n8nClient *n8n.Client) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Get effective sync policy (default to Always)
	syncPolicy := workflow.Spec.SyncPolicy
	if syncPolicy == "" {
		syncPolicy = n8nv1alpha1.SyncPolicyAlways
	}

	// Handle Manual sync policy - skip all sync operations
	if syncPolicy == n8nv1alpha1.SyncPolicyManual {
		log.Info("SyncPolicy is Manual, skipping reconciliation")
		r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionTrue,
			"SyncPaused", "Sync is paused (syncPolicy: Manual)")
		if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: defaultRequeueInterval}, nil
	}

	// Convert CRD workflow spec to n8n workflow
	n8nWorkflow, err := r.convertToN8nWorkflow(workflow)
	if err != nil {
		log.Error(err, "Failed to convert workflow spec")
		r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
			n8nv1alpha1.ReasonSyncFailed, fmt.Sprintf("Failed to convert workflow: %v", err))
		if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
	}

	var existingWorkflow *n8n.Workflow

	// Check if workflow already exists in n8n
	if workflow.Status.WorkflowID != "" {
		// Try to get by ID first
		existingWorkflow, err = n8nClient.GetWorkflow(ctx, workflow.Status.WorkflowID)
		if err != nil {
			log.Info("Failed to get workflow by ID, will search by name", "id", workflow.Status.WorkflowID, "error", err)
			existingWorkflow = nil
		}
	}

	// If not found by ID, search by name
	if existingWorkflow == nil {
		existingWorkflow, err = n8nClient.GetWorkflowByName(ctx, workflow.Spec.Workflow.Name)
		if err != nil {
			log.Error(err, "Failed to search workflow by name")
			r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
				n8nv1alpha1.ReasonAPIError, fmt.Sprintf("Failed to search workflow: %v", err))
			if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}
	}

	if existingWorkflow == nil {
		// Create new workflow
		log.Info("Creating new workflow in n8n", "name", workflow.Spec.Workflow.Name)
		created, err := n8nClient.CreateWorkflow(ctx, n8nWorkflow)
		if err != nil {
			log.Error(err, "Failed to create workflow")
			r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
				n8nv1alpha1.ReasonSyncFailed, fmt.Sprintf("Failed to create workflow: %v", err))
			r.Recorder.Event(workflow, corev1.EventTypeWarning, "CreateFailed", err.Error())
			if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}
		workflow.Status.WorkflowID = created.ID
		r.Recorder.Event(workflow, corev1.EventTypeNormal, "Created", fmt.Sprintf("Workflow created with ID %s", created.ID))
		existingWorkflow = created
	} else {
		// Workflow exists - check sync policy before updating
		workflow.Status.WorkflowID = existingWorkflow.ID

		if syncPolicy == n8nv1alpha1.SyncPolicyCreateOnly {
			// CreateOnly: Don't update, just track the workflow
			log.Info("SyncPolicy is CreateOnly, skipping update", "id", existingWorkflow.ID)
		} else {
			// Always: Update the workflow
			log.Info("Updating workflow in n8n", "id", existingWorkflow.ID, "name", workflow.Spec.Workflow.Name)

			// Check if update is needed
			if r.needsUpdate(existingWorkflow, n8nWorkflow) {
				updated, err := n8nClient.UpdateWorkflow(ctx, existingWorkflow.ID, n8nWorkflow)
				if err != nil {
					log.Error(err, "Failed to update workflow")
					r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
						n8nv1alpha1.ReasonSyncFailed, fmt.Sprintf("Failed to update workflow: %v", err))
					r.Recorder.Event(workflow, corev1.EventTypeWarning, "UpdateFailed", err.Error())
					if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
						log.Error(statusErr, "Failed to update status")
					}
					return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
				}
				r.Recorder.Event(workflow, corev1.EventTypeNormal, "Updated", "Workflow updated successfully")
				existingWorkflow = updated
			}
		}
	}

	// Handle activation/deactivation
	if workflow.Spec.Active && !existingWorkflow.Active {
		log.Info("Activating workflow", "id", workflow.Status.WorkflowID)
		activated, err := n8nClient.ActivateWorkflow(ctx, workflow.Status.WorkflowID)
		if err != nil {
			log.Error(err, "Failed to activate workflow")
			r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
				n8nv1alpha1.ReasonActivationError, fmt.Sprintf("Failed to activate workflow: %v", err))
			r.Recorder.Event(workflow, corev1.EventTypeWarning, "ActivationFailed", err.Error())
			if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}
		workflow.Status.Active = true
		r.Recorder.Event(workflow, corev1.EventTypeNormal, "Activated", "Workflow activated successfully")
		existingWorkflow = activated
	} else if !workflow.Spec.Active && existingWorkflow.Active {
		log.Info("Deactivating workflow", "id", workflow.Status.WorkflowID)
		deactivated, err := n8nClient.DeactivateWorkflow(ctx, workflow.Status.WorkflowID)
		if err != nil {
			log.Error(err, "Failed to deactivate workflow")
			r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionFalse,
				n8nv1alpha1.ReasonActivationError, fmt.Sprintf("Failed to deactivate workflow: %v", err))
			r.Recorder.Event(workflow, corev1.EventTypeWarning, "DeactivationFailed", err.Error())
			if statusErr := r.Status().Update(ctx, workflow); statusErr != nil {
				log.Error(statusErr, "Failed to update status")
			}
			return ctrl.Result{RequeueAfter: errorRequeueInterval}, err
		}
		workflow.Status.Active = false
		r.Recorder.Event(workflow, corev1.EventTypeNormal, "Deactivated", "Workflow deactivated successfully")
		existingWorkflow = deactivated
	} else {
		workflow.Status.Active = existingWorkflow.Active
	}

	// Extract webhook URL if present
	workflow.Status.WebhookURL = r.extractWebhookURL(existingWorkflow)

	// Update status
	now := metav1.Now()
	workflow.Status.LastSyncTime = &now
	workflow.Status.ObservedGeneration = workflow.Generation

	r.setCondition(workflow, n8nv1alpha1.ConditionTypeReady, metav1.ConditionTrue,
		n8nv1alpha1.ReasonSyncSucceeded, "Workflow synced successfully")
	r.setCondition(workflow, n8nv1alpha1.ConditionTypeSynced, metav1.ConditionTrue,
		n8nv1alpha1.ReasonSyncSucceeded, "Workflow synced to n8n")

	if err := r.Status().Update(ctx, workflow); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Reconciliation complete", "workflowId", workflow.Status.WorkflowID, "active", workflow.Status.Active)
	return ctrl.Result{RequeueAfter: defaultRequeueInterval}, nil
}

// handleDeletion handles the deletion of an N8nWorkflow
func (r *N8nWorkflowReconciler) handleDeletion(ctx context.Context, workflow *n8nv1alpha1.N8nWorkflow, n8nClient *n8n.Client) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	if !controllerutil.ContainsFinalizer(workflow, finalizerName) {
		return ctrl.Result{}, nil
	}

	log.Info("Handling deletion of N8nWorkflow")

	// Delete the workflow from n8n if it exists
	if workflow.Status.WorkflowID != "" {
		log.Info("Deleting workflow from n8n", "id", workflow.Status.WorkflowID)
		err := n8nClient.DeleteWorkflow(ctx, workflow.Status.WorkflowID)
		if err != nil {
			// Check if the workflow was already deleted (not found is acceptable)
			if strings.Contains(err.Error(), "Not Found") || strings.Contains(err.Error(), "not found") {
				log.Info("Workflow already deleted from n8n", "id", workflow.Status.WorkflowID)
			} else {
				// Log as warning but continue with finalizer removal
				log.Info("Failed to delete workflow from n8n (continuing with cleanup)", "error", err)
				r.Recorder.Event(workflow, corev1.EventTypeWarning, "DeleteFailed",
					fmt.Sprintf("Failed to delete workflow from n8n: %v", err))
			}
		} else {
			r.Recorder.Event(workflow, corev1.EventTypeNormal, "Deleted", "Workflow deleted from n8n")
		}
	}

	// Remove finalizer
	controllerutil.RemoveFinalizer(workflow, finalizerName)
	if err := r.Update(ctx, workflow); err != nil {
		log.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	log.Info("Successfully deleted N8nWorkflow")
	return ctrl.Result{}, nil
}

// convertToN8nWorkflow converts the CRD spec to an n8n API workflow
func (r *N8nWorkflowReconciler) convertToN8nWorkflow(workflow *n8nv1alpha1.N8nWorkflow) (*n8n.Workflow, error) {
	n8nWorkflow := &n8n.Workflow{
		Name:   workflow.Spec.Workflow.Name,
		Active: workflow.Spec.Active,
	}

	// Convert nodes
	if len(workflow.Spec.Workflow.Nodes) > 0 {
		n8nWorkflow.Nodes = make([]map[string]any, len(workflow.Spec.Workflow.Nodes))
		for i, node := range workflow.Spec.Workflow.Nodes {
			var nodeMap map[string]any
			if err := json.Unmarshal(node.Raw, &nodeMap); err != nil {
				return nil, fmt.Errorf("failed to unmarshal node %d: %w", i, err)
			}
			n8nWorkflow.Nodes[i] = nodeMap
		}
	}

	// Convert connections
	if workflow.Spec.Workflow.Connections != nil && workflow.Spec.Workflow.Connections.Raw != nil {
		var connections map[string]any
		if err := json.Unmarshal(workflow.Spec.Workflow.Connections.Raw, &connections); err != nil {
			return nil, fmt.Errorf("failed to unmarshal connections: %w", err)
		}
		n8nWorkflow.Connections = connections
	}

	// Convert settings
	if workflow.Spec.Workflow.Settings != nil && workflow.Spec.Workflow.Settings.Raw != nil {
		var settings map[string]any
		if err := json.Unmarshal(workflow.Spec.Workflow.Settings.Raw, &settings); err != nil {
			return nil, fmt.Errorf("failed to unmarshal settings: %w", err)
		}
		n8nWorkflow.Settings = settings
	}

	// Convert static data
	if workflow.Spec.Workflow.StaticData != nil && workflow.Spec.Workflow.StaticData.Raw != nil {
		var staticData map[string]any
		if err := json.Unmarshal(workflow.Spec.Workflow.StaticData.Raw, &staticData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal staticData: %w", err)
		}
		n8nWorkflow.StaticData = staticData
	}

	// Convert pin data
	if workflow.Spec.Workflow.PinData != nil && workflow.Spec.Workflow.PinData.Raw != nil {
		var pinData map[string]any
		if err := json.Unmarshal(workflow.Spec.Workflow.PinData.Raw, &pinData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal pinData: %w", err)
		}
		n8nWorkflow.PinData = pinData
	}

	return n8nWorkflow, nil
}

// needsUpdate checks if the workflow needs to be updated in n8n
func (r *N8nWorkflowReconciler) needsUpdate(existing *n8n.Workflow, desired *n8n.Workflow) bool {
	// For now, always update to ensure consistency
	// In a production system, you might want to compare the actual content
	return true
}

// extractWebhookURL extracts the webhook URL from a workflow if it has a webhook trigger
func (r *N8nWorkflowReconciler) extractWebhookURL(workflow *n8n.Workflow) string {
	if workflow == nil || len(workflow.Nodes) == 0 {
		return ""
	}

	for _, node := range workflow.Nodes {
		nodeType, ok := node["type"].(string)
		if !ok {
			continue
		}
		if nodeType == "n8n-nodes-base.webhook" {
			params, ok := node["parameters"].(map[string]any)
			if !ok {
				continue
			}
			path, ok := params["path"].(string)
			if ok {
				return "/webhook/" + path
			}
		}
	}

	return ""
}

// setCondition sets a condition on the workflow status
func (r *N8nWorkflowReconciler) setCondition(workflow *n8nv1alpha1.N8nWorkflow, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: workflow.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&workflow.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *N8nWorkflowReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&n8nv1alpha1.N8nWorkflow{}).
		Named("n8nworkflow").
		Complete(r)
}
