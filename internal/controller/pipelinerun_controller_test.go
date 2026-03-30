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

var _ = Describe("PipelineRun Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default", // TODO(user):Modify as needed
		}
		pipelinerun := &aiv1alpha1.PipelineRun{}

		BeforeEach(func() {
			By("creating the custom resource for the Kind PipelineRun")
			err := k8sClient.Get(ctx, typeNamespacedName, pipelinerun)
			if err != nil && errors.IsNotFound(err) {
				resource := &aiv1alpha1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      resourceName,
						Namespace: "default",
					},
					// TODO(user): Specify other spec details if needed.
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			// TODO(user): Cleanup logic after each test, like removing the resource instance.
			resource := &aiv1alpha1.PipelineRun{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance PipelineRun")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should successfully reconcile the resource", func() {
			By("Reconciling the created resource")
			controllerReconciler := &PipelineRunReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Context("When reconciling a spot run", func() {
		const spotRunName = "test-spot-run"

		ctx := context.Background()

		spotKey := types.NamespacedName{
			Name:      spotRunName,
			Namespace: "default",
		}

		BeforeEach(func() {
			resource := &aiv1alpha1.PipelineRun{}
			err := k8sClient.Get(ctx, spotKey, resource)
			if err != nil && errors.IsNotFound(err) {
				resource = &aiv1alpha1.PipelineRun{
					ObjectMeta: metav1.ObjectMeta{
						Name:      spotRunName,
						Namespace: "default",
						Labels: map[string]string{
							"ai.aipipelines.io/spot": "true",
						},
					},
					Spec: aiv1alpha1.PipelineRunSpec{
						PipelineRef: "test-pipeline",
						Description: "Fix the login bug on the settings page",
					},
				}
				Expect(k8sClient.Create(ctx, resource)).To(Succeed())
			}
		})

		AfterEach(func() {
			resource := &aiv1alpha1.PipelineRun{}
			err := k8sClient.Get(ctx, spotKey, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should create a spot run without issue fields", func() {
			var run aiv1alpha1.PipelineRun
			Expect(k8sClient.Get(ctx, spotKey, &run)).To(Succeed())
			Expect(run.Spec.Description).To(Equal("Fix the login bug on the settings page"))
			Expect(run.Spec.IssueNumber).To(Equal(0))
			Expect(run.Spec.IssueKey).To(BeEmpty())
			Expect(run.Spec.IssueTitle).To(BeEmpty())
		})
	})
})
