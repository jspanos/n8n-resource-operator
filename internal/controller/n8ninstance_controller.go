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
	"fmt"
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
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	n8nv1alpha1 "github.com/jspanos/n8n-resource-operator/api/v1alpha1"
	"github.com/jspanos/n8n-resource-operator/internal/n8n"
)

const (
	// Health check interval
	healthCheckInterval = 5 * time.Minute

	// Error requeue interval for instance
	instanceErrorRequeueInterval = 30 * time.Second
)

// N8nInstanceReconciler reconciles a N8nInstance object
type N8nInstanceReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=n8n.slys.dev,resources=n8ninstances,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=n8n.slys.dev,resources=n8ninstances/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=n8n.slys.dev,resources=n8ninstances/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// Reconcile is part of the main kubernetes reconciliation loop
func (r *N8nInstanceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.V(1).Info("Reconciling N8nInstance")

	// Fetch the N8nInstance
	instance := &n8nv1alpha1.N8nInstance{}
	if err := r.Get(ctx, req.NamespacedName, instance); err != nil {
		if errors.IsNotFound(err) {
			log.Info("N8nInstance resource not found, ignoring")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get N8nInstance")
		return ctrl.Result{}, err
	}

	// Validate configuration
	if err := r.validateInstance(instance); err != nil {
		log.Error(err, "Invalid N8nInstance configuration")
		r.setCondition(instance, n8nv1alpha1.InstanceConditionTypeReady, metav1.ConditionFalse,
			n8nv1alpha1.InstanceReasonInvalidConfig, err.Error())
		instance.Status.Ready = false
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: instanceErrorRequeueInterval}, nil
	}

	// Resolve URL
	resolvedURL := instance.GetResolvedURL()
	instance.Status.URL = resolvedURL

	// Get API key from secret
	apiKey, err := r.getAPIKey(ctx, instance)
	if err != nil {
		log.Error(err, "Failed to get API key from secret")
		r.setCondition(instance, n8nv1alpha1.InstanceConditionTypeReady, metav1.ConditionFalse,
			n8nv1alpha1.InstanceReasonAuthError, fmt.Sprintf("Failed to get API key: %v", err))
		instance.Status.Ready = false
		r.Recorder.Event(instance, corev1.EventTypeWarning, "SecretError", err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: instanceErrorRequeueInterval}, nil
	}

	// Create n8n client and perform health check
	n8nClient := n8n.NewClient(resolvedURL, apiKey)
	if err := n8nClient.HealthCheck(ctx); err != nil {
		log.Error(err, "Health check failed")
		r.setCondition(instance, n8nv1alpha1.InstanceConditionTypeReady, metav1.ConditionFalse,
			n8nv1alpha1.InstanceReasonConnectionError, fmt.Sprintf("Health check failed: %v", err))
		instance.Status.Ready = false
		r.Recorder.Event(instance, corev1.EventTypeWarning, "HealthCheckFailed", err.Error())
		if statusErr := r.Status().Update(ctx, instance); statusErr != nil {
			log.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: instanceErrorRequeueInterval}, nil
	}

	// Health check passed - update status
	now := metav1.Now()
	instance.Status.Ready = true
	instance.Status.LastHealthCheck = &now
	instance.Status.ObservedGeneration = instance.Generation

	r.setCondition(instance, n8nv1alpha1.InstanceConditionTypeReady, metav1.ConditionTrue,
		n8nv1alpha1.InstanceReasonConnected, "Successfully connected to n8n instance")

	if err := r.Status().Update(ctx, instance); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.V(1).Info("N8nInstance reconciliation complete", "url", resolvedURL, "ready", true)
	return ctrl.Result{RequeueAfter: healthCheckInterval}, nil
}

// validateInstance validates the N8nInstance configuration
func (r *N8nInstanceReconciler) validateInstance(instance *n8nv1alpha1.N8nInstance) error {
	// Either URL or ServiceRef must be specified
	hasURL := instance.Spec.URL != ""
	hasServiceRef := instance.Spec.ServiceRef != nil

	if !hasURL && !hasServiceRef {
		return fmt.Errorf("either url or serviceRef must be specified")
	}

	if hasURL && hasServiceRef {
		return fmt.Errorf("only one of url or serviceRef can be specified, not both")
	}

	// Validate ServiceRef if specified
	if hasServiceRef {
		if instance.Spec.ServiceRef.Name == "" {
			return fmt.Errorf("serviceRef.name is required")
		}
		if instance.Spec.ServiceRef.Namespace == "" {
			return fmt.Errorf("serviceRef.namespace is required")
		}
	}

	// Credentials must be specified
	if instance.Spec.Credentials.SecretName == "" {
		return fmt.Errorf("credentials.secretName is required")
	}

	return nil
}

// getAPIKey retrieves the API key from the referenced secret
func (r *N8nInstanceReconciler) getAPIKey(ctx context.Context, instance *n8nv1alpha1.N8nInstance) (string, error) {
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{
		Name:      instance.Spec.Credentials.SecretName,
		Namespace: instance.Namespace, // Secret must be in same namespace as N8nInstance
	}

	if err := r.Get(ctx, secretKey, secret); err != nil {
		return "", fmt.Errorf("failed to get secret %s/%s: %w", secretKey.Namespace, secretKey.Name, err)
	}

	key := instance.GetSecretKey()
	apiKeyBytes, ok := secret.Data[key]
	if !ok {
		return "", fmt.Errorf("secret %s/%s does not contain key %s", secretKey.Namespace, secretKey.Name, key)
	}

	return string(apiKeyBytes), nil
}

// setCondition sets a condition on the instance status
func (r *N8nInstanceReconciler) setCondition(instance *n8nv1alpha1.N8nInstance, conditionType string, status metav1.ConditionStatus, reason, message string) {
	condition := metav1.Condition{
		Type:               conditionType,
		Status:             status,
		ObservedGeneration: instance.Generation,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}
	meta.SetStatusCondition(&instance.Status.Conditions, condition)
}

// SetupWithManager sets up the controller with the Manager.
func (r *N8nInstanceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&n8nv1alpha1.N8nInstance{}).
		Named("n8ninstance").
		Complete(r)
}
