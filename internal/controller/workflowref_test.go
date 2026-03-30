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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("WorkflowRef in StepSpec", func() {
	Context("When creating a Pipeline with workflowRef", func() {
		const resourceName = "test-workflowref-pipeline"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			resource := &aiv1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should accept a step with workflowRef containing repo, path, and ref", func() {
			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					AI: aiv1alpha1.AISpec{
						Image: "test-image:latest",
					},
					Steps: []aiv1alpha1.StepSpec{
						{
							Name: "fix-bug",
							Type: "ai",
							WorkflowRef: &aiv1alpha1.WorkflowRef{
								Repo: "ambient-code/workflows",
								Path: "bugfix",
								Ref:  "main",
							},
							PromptTemplate: "Fix the bug described in issue #123",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			Expect(fetched.Spec.Steps).To(HaveLen(1))
			Expect(fetched.Spec.Steps[0].WorkflowRef).NotTo(BeNil())
			Expect(fetched.Spec.Steps[0].WorkflowRef.Repo).To(Equal("ambient-code/workflows"))
			Expect(fetched.Spec.Steps[0].WorkflowRef.Path).To(Equal("bugfix"))
			Expect(fetched.Spec.Steps[0].WorkflowRef.Ref).To(Equal("main"))
			Expect(fetched.Spec.Steps[0].PromptTemplate).To(Equal("Fix the bug described in issue #123"))
		})

		It("should accept a step with workflowRef without ref (defaults to HEAD)", func() {
			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					AI: aiv1alpha1.AISpec{
						Image: "test-image:latest",
					},
					Steps: []aiv1alpha1.StepSpec{
						{
							Name: "fix-bug",
							Type: "ai",
							WorkflowRef: &aiv1alpha1.WorkflowRef{
								Repo: "ambient-code/workflows",
								Path: "bugfix",
							},
							PromptTemplate: "Fix the bug described in issue #123",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			Expect(fetched.Spec.Steps[0].WorkflowRef).NotTo(BeNil())
			Expect(fetched.Spec.Steps[0].WorkflowRef.Repo).To(Equal("ambient-code/workflows"))
			Expect(fetched.Spec.Steps[0].WorkflowRef.Path).To(Equal("bugfix"))
			Expect(fetched.Spec.Steps[0].WorkflowRef.Ref).To(BeEmpty())
		})

		It("should accept a step without workflowRef (backward compatible)", func() {
			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					AI: aiv1alpha1.AISpec{
						Image: "test-image:latest",
					},
					Steps: []aiv1alpha1.StepSpec{
						{
							Name:           "fix-bug",
							Type:           "ai",
							PromptTemplate: "Full methodology and instructions here",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			Expect(fetched.Spec.Steps[0].WorkflowRef).To(BeNil())
			Expect(fetched.Spec.Steps[0].PromptTemplate).To(Equal("Full methodology and instructions here"))
		})
	})

	Context("WorkflowRef struct fields", func() {
		It("should have the required Repo field", func() {
			ref := aiv1alpha1.WorkflowRef{
				Repo: "ambient-code/workflows",
				Path: "bugfix",
			}
			Expect(ref.Repo).To(Equal("ambient-code/workflows"))
		})

		It("should have the required Path field", func() {
			ref := aiv1alpha1.WorkflowRef{
				Repo: "ambient-code/workflows",
				Path: "bugfix",
			}
			Expect(ref.Path).To(Equal("bugfix"))
		})

		It("should have the optional Ref field", func() {
			ref := aiv1alpha1.WorkflowRef{
				Repo: "ambient-code/workflows",
				Path: "bugfix",
				Ref:  "v1.0.0",
			}
			Expect(ref.Ref).To(Equal("v1.0.0"))
		})
	})

	Context("When using workflowRef with promptTemplate", func() {
		const resourceName = "test-workflowref-prompt-pipeline"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			resource := &aiv1alpha1.Pipeline{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should use promptTemplate as issue/task context when workflowRef is set", func() {
			issueContext := "Issue #42: Login button is broken on mobile Safari"
			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					AI: aiv1alpha1.AISpec{
						Image: "test-image:latest",
					},
					Steps: []aiv1alpha1.StepSpec{
						{
							Name: "fix-bug",
							Type: "ai",
							WorkflowRef: &aiv1alpha1.WorkflowRef{
								Repo: "ambient-code/workflows",
								Path: "bugfix",
								Ref:  "main",
							},
							PromptTemplate: issueContext,
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			// When workflowRef is set, promptTemplate serves as the issue/task context
			// (the initial user message), not methodology instructions
			step := fetched.Spec.Steps[0]
			Expect(step.WorkflowRef).NotTo(BeNil())
			Expect(step.PromptTemplate).To(Equal(issueContext))
		})
	})

	Context("WorkflowRef serialization", func() {
		It("should serialize workflowRef to JSON with correct field names", func() {
			// Verify the struct has correct json tags by checking round-trip through K8s API
			const resourceName = "test-workflowref-json"

			ctx := context.Background()

			typeNamespacedName := types.NamespacedName{
				Name:      resourceName,
				Namespace: "default",
			}

			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					AI: aiv1alpha1.AISpec{
						Image: "test-image:latest",
					},
					Steps: []aiv1alpha1.StepSpec{
						{
							Name: "ai-step",
							Type: "ai",
							WorkflowRef: &aiv1alpha1.WorkflowRef{
								Repo: "my-org/my-workflows",
								Path: "code-review",
								Ref:  "v2.0.0",
							},
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			wfRef := fetched.Spec.Steps[0].WorkflowRef
			Expect(wfRef).NotTo(BeNil())
			Expect(wfRef.Repo).To(Equal("my-org/my-workflows"))
			Expect(wfRef.Path).To(Equal("code-review"))
			Expect(wfRef.Ref).To(Equal("v2.0.0"))

			// Cleanup
			Expect(k8sClient.Delete(ctx, &fetched)).To(Succeed())

			// Verify deleted
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &aiv1alpha1.Pipeline{})
				return errors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})
})
