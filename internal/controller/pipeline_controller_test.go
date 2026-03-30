/*
Copyright 2026.

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
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("Pipeline Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		pipeline := &aiv1alpha1.Pipeline{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind Pipeline")
			err := k8sClient.Get(ctx, typeNamespacedName, pipeline)
			if err != nil && errors.IsNotFound(err) {
				resource := &aiv1alpha1.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					Spec: aiv1alpha1.PipelineSpec{
						Trigger: &aiv1alpha1.TriggerSpec{
							GitHub: &aiv1alpha1.GitHubTriggerSpec{
								Owner:    "test-owner",
								Repo:     "test-repo",
								Assignee: "test-user",
								SecretRef: aiv1alpha1.SecretKeyRef{
									Name: "test-secret",
								},
							},
						},
						AI: aiv1alpha1.AISpec{
							Image: "test-image:latest",
						},
						Steps: []aiv1alpha1.StepSpec{
							{
								Name: "checkout",
								Type: "git-checkout",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &aiv1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance Pipeline")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a spot (triggerless) pipeline", func() {
		const spotName = "test-spot-pipeline"

		ctx := context.Background()

		spotKey := types.NamespacedName{
			Name:      spotName,
			Namespace: "default",
		}

		BeforeEach(func() {
			resource := &aiv1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, spotKey, resource)
			if err != nil && errors.IsNotFound(err) {
				resource = &aiv1alpha1.Pipeline{
					ObjectMeta: metav1.ObjectMeta{
						Name:      spotName,
						Namespace: "default",
					},
					Spec: aiv1alpha1.PipelineSpec{
						// No Trigger — spot-run-only pipeline
						AI: aiv1alpha1.AISpec{
							Image: "test-image:latest",
						},
						Steps: []aiv1alpha1.StepSpec{
							{
								Name: "checkout",
								Type: "git-checkout",
							},
						},
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &aiv1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, spotKey, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should reconcile without starting a poller", func() {
			controllerReconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: spotKey,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// Verify no poller was started
			var updated aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, spotKey, &updated)).To(Succeed())
			Expect(updated.Status.PollerActive).To(BeFalse())
		})
	})
})
