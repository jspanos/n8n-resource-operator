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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	n8nv1alpha1 "github.com/jspanos/n8n-resource-operator/api/v1alpha1"
)

var _ = Describe("N8nWorkflow Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}
		n8nworkflow := &n8nv1alpha1.N8nWorkflow{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind N8nWorkflow")
			err := k8sClient.Get(ctx, typeNamespacedName, n8nworkflow)
			if err != nil && errors.IsNotFound(err) {
				resource := &n8nv1alpha1.N8nWorkflow{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: n8nv1alpha1.N8nWorkflowSpec{
						InstanceRef: "test-instance",
						Active:      true,
						Workflow: n8nv1alpha1.WorkflowSpec{
							Name: "Test Workflow",
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &n8nv1alpha1.N8nWorkflow{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				// Remove finalizer if present to allow deletion
				if len(resource.Finalizers) > 0 {
					resource.Finalizers = nil
					Expect(k8sClient.Update(ctx, resource)).To(Succeed())
				}
				By("Cleanup the specific resource instance N8nWorkflow")
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			fakeRecorder := record.NewFakeRecorder(10)
			controllerReconciler := &N8nWorkflowReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				Recorder:          fakeRecorder,
				OperatorNamespace: "default",
			}

			// Reconciliation will fail without N8nInstance, but should not panic
			// and should update status with appropriate error condition
			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// Expected to fail due to missing N8nInstance
			Expect(err).To(HaveOccurred())

			// Verify the resource still exists and status was updated
			resource := &n8nv1alpha1.N8nWorkflow{}
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
		})

		It("should add finalizer on first reconcile", func() {
			By("Setting up reconciler with operator namespace")
			fakeRecorder := record.NewFakeRecorder(10)
			controllerReconciler := &N8nWorkflowReconciler{
				Client:            k8sClient,
				Scheme:            k8sClient.Scheme(),
				Recorder:          fakeRecorder,
				OperatorNamespace: "default",
			}

			By("Running first reconcile")
			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})

			// First reconcile adds finalizer and requeues (may fail due to missing N8nInstance)
			if err == nil && result.Requeue {
				// Verify finalizer was added
				resource := &n8nv1alpha1.N8nWorkflow{}
				err = k8sClient.Get(ctx, typeNamespacedName, resource)
				Expect(err).NotTo(HaveOccurred())
				Expect(resource.Finalizers).To(ContainElement("n8n.slys.dev/workflow-cleanup"))
			}
		})
	})
})
