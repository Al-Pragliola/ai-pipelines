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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("Pipeline Controller - GitHubPRReview Trigger", func() {
	Context("When creating a Pipeline with a GitHubPRReview trigger", func() {
		const resourceName = "test-pr-review-pipeline"

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

		It("should accept a Pipeline with GitHubPRReview trigger fields", func() {
			resource := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:        "test-owner",
							Repo:         "test-repo",
							Reviewer:     "test-reviewer",
							PollInterval: "60s",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "pr-review-secret",
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

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())
			Expect(fetched.Spec.Trigger).NotTo(BeNil())
			Expect(fetched.Spec.Trigger.GitHubPRReview).NotTo(BeNil())
			Expect(fetched.Spec.Trigger.GitHubPRReview.Owner).To(Equal("test-owner"))
			Expect(fetched.Spec.Trigger.GitHubPRReview.Repo).To(Equal("test-repo"))
			Expect(fetched.Spec.Trigger.GitHubPRReview.Reviewer).To(Equal("test-reviewer"))
			Expect(fetched.Spec.Trigger.GitHubPRReview.PollInterval).To(Equal("60s"))
			Expect(fetched.Spec.Trigger.GitHubPRReview.SecretRef.Name).To(Equal("pr-review-secret"))
		})

		It("should reconcile a Pipeline with GitHubPRReview trigger", func() {
			resource := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:    "review-owner",
							Repo:     "review-repo",
							Reviewer: "review-user",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "review-secret",
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

			controllerReconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
		})

		It("should store the secretRef key with default value", func() {
			resource := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:    "owner",
							Repo:     "repo",
							Reviewer: "reviewer",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "secret-name",
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

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())
			Expect(fetched.Spec.Trigger.GitHubPRReview.SecretRef.Key).To(Equal("token"))
		})

		It("should not conflict with other trigger types in TriggerSpec", func() {
			resource := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:    "owner",
							Repo:     "repo",
							Reviewer: "reviewer",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "secret",
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

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())
			Expect(fetched.Spec.Trigger.GitHub).To(BeNil())
			Expect(fetched.Spec.Trigger.Jira).To(BeNil())
			Expect(fetched.Spec.Trigger.GitHubPRReview).NotTo(BeNil())
		})
	})

	Context("When creating a Pipeline without GitHubPRReview trigger", func() {
		const resourceName = "test-no-pr-review-pipeline"

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

		It("should have nil GitHubPRReview when using GitHub trigger", func() {
			resource := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHub: &aiv1alpha1.GitHubTriggerSpec{
							Owner:    "owner",
							Repo:     "repo",
							Assignee: "user",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "secret",
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

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())
			Expect(fetched.Spec.Trigger.GitHubPRReview).To(BeNil())
		})
	})
})

// Compile-time assertion that GitHubPRReviewTriggerSpec has the expected fields.
var _ = func() {
	_ = aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:        "owner",
		Repo:         "repo",
		Reviewer:     "reviewer",
		PollInterval: "30s",
		SecretRef:    aiv1alpha1.SecretKeyRef{Name: "secret"},
	}

	// Verify it can be assigned to TriggerSpec.GitHubPRReview
	_ = aiv1alpha1.TriggerSpec{
		GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{},
	}
}
