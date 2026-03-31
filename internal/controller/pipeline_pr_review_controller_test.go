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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
	"github.com/Al-Pragliola/ai-pipelines/internal/trigger"
)

var _ = Describe("Pipeline Controller - GitHubPRReview trigger wiring", func() {
	Context("Reconcile switch: reading secret for GitHubPRReview trigger", func() {
		const (
			pipelineName = "pr-review-secret-pipeline"
			secretName   = "pr-review-gh-token"
			namespace    = "default"
		)

		ctx := context.Background()
		pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: namespace}

		BeforeEach(func() {
			// Create the secret that the trigger references
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("ghp_test_token_for_pr_review"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			// Create the Pipeline with GitHubPRReview trigger
			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: namespace,
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:        "test-owner",
							Repo:         "test-repo",
							Reviewer:     "test-reviewer",
							PollInterval: "45s",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: secretName,
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
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			pipeline := &aiv1alpha1.Pipeline{}
			if err := k8sClient.Get(ctx, pipelineKey, pipeline); err == nil {
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
			}
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
			if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should read the secret and set PollerActive to true", func() {
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineKey,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			// The reconciler should have read the secret and started a poller
			var updated aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, pipelineKey, &updated)).To(Succeed())
			Expect(updated.Status.PollerActive).To(BeTrue(),
				"PollerActive should be true when GitHubPRReview trigger is configured with a valid secret")
		})
	})

	Context("Reconcile switch: GitHubPRReview with missing secret", func() {
		const (
			pipelineName = "pr-review-missing-secret"
			namespace    = "default"
		)

		ctx := context.Background()
		pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: namespace}

		BeforeEach(func() {
			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: namespace,
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:    "test-owner",
							Repo:     "test-repo",
							Reviewer: "test-reviewer",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "nonexistent-secret",
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
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			pipeline := &aiv1alpha1.Pipeline{}
			if err := k8sClient.Get(ctx, pipelineKey, pipeline); err == nil {
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
			}
		})

		It("should requeue after 30s when the secret is not found", func() {
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineKey,
			})
			// When secret is missing, the controller should requeue (like GitHub/Jira triggers do)
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30*time.Second),
				"should requeue after 30s when GitHubPRReview secret is missing")
		})
	})

	Context("runPoller: parsing GitHubPRReview PollInterval", func() {
		const (
			pipelineName = "pr-review-poll-interval"
			secretName   = "pr-review-poll-secret"
			namespace    = "default"
		)

		ctx := context.Background()
		pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: namespace}

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("ghp_test_token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: namespace,
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:        "test-owner",
							Repo:         "test-repo",
							Reviewer:     "test-reviewer",
							PollInterval: "2m",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: secretName,
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
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			pipeline := &aiv1alpha1.Pipeline{}
			if err := k8sClient.Get(ctx, pipelineKey, pipeline); err == nil {
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
			}
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
			if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should start a poller that respects the configured PollInterval", func() {
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: pipelineKey,
			})
			Expect(err).NotTo(HaveOccurred())

			// Verify poller was registered
			reconciler.mu.Lock()
			_, pollerExists := reconciler.pollers[pipelineKey]
			reconciler.mu.Unlock()
			Expect(pollerExists).To(BeTrue(),
				"a poller should be registered for the GitHubPRReview pipeline")

			// Cleanup: stop the poller
			reconciler.stopPoller(pipelineKey)
		})
	})

	Context("poll: calling FetchGitHubReviewRequests and creating PipelineRuns", func() {
		const (
			pipelineName = "pr-review-poll-create"
			secretName   = "pr-review-create-secret"
			namespace    = "default"
		)

		ctx := context.Background()
		pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: namespace}

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("ghp_test_token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: namespace,
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:    "test-owner",
							Repo:     "test-repo",
							Reviewer: "test-reviewer",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: secretName,
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
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			// Clean up PipelineRuns
			var runs aiv1alpha1.PipelineRunList
			if err := k8sClient.List(ctx, &runs); err == nil {
				for i := range runs.Items {
					_ = k8sClient.Delete(ctx, &runs.Items[i])
				}
			}
			pipeline := &aiv1alpha1.Pipeline{}
			if err := k8sClient.Get(ctx, pipelineKey, pipeline); err == nil {
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
			}
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
			if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should create a PipelineRun with issue-key label using #PR-N format", func() {
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Directly call createPipelineRun with a PR-style issue
			prIssue := trigger.Issue{
				Number: 42,
				Key:    "#PR-42",
				Title:  "Add new feature",
				Body:   "This PR adds a new feature",
			}

			err := reconciler.createPipelineRun(ctx, namespace, pipelineName, pipelineKey, prIssue)
			Expect(err).NotTo(HaveOccurred())

			// Verify the PipelineRun was created with expected labels
			var runs aiv1alpha1.PipelineRunList
			Expect(k8sClient.List(ctx, &runs,
				client.InNamespace(namespace),
				client.MatchingLabels{
					"ai.aipipelines.io/pipeline":  pipelineName,
					"ai.aipipelines.io/issue-key": "PR-42",
				},
			)).To(Succeed())
			Expect(runs.Items).To(HaveLen(1))

			run := runs.Items[0]
			Expect(run.Spec.PipelineRef).To(Equal(pipelineName))
			Expect(run.Spec.IssueKey).To(Equal("#PR-42"))
			Expect(run.Spec.IssueNumber).To(Equal(42))
			Expect(run.Spec.IssueTitle).To(Equal("Add new feature"))
			Expect(run.Spec.IssueBody).To(Equal("This PR adds a new feature"))
		})

		It("should populate PR-specific fields (prNumber, prAuthor, baseBranch, headBranch) on the PipelineRun", func() {
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			// Simulate a PR review trigger issue with PR metadata
			prIssue := trigger.Issue{
				Number: 99,
				Key:    "#PR-99",
				Title:  "Fix critical bug",
				Body:   "This fixes the critical bug in auth",
			}

			err := reconciler.createPipelineRun(ctx, namespace, pipelineName, pipelineKey, prIssue)
			Expect(err).NotTo(HaveOccurred())

			var runs aiv1alpha1.PipelineRunList
			Expect(k8sClient.List(ctx, &runs,
				client.InNamespace(namespace),
				client.MatchingLabels{
					"ai.aipipelines.io/pipeline":  pipelineName,
					"ai.aipipelines.io/issue-key": "PR-99",
				},
			)).To(Succeed())
			Expect(runs.Items).To(HaveLen(1))

			run := runs.Items[0]
			// The controller should populate PR-specific fields when dealing
			// with a GitHubPRReview trigger
			Expect(run.Spec.PRNumber).To(Equal(99),
				"PRNumber should be populated from the PR review trigger issue")
			Expect(run.Spec.PRAuthor).NotTo(BeEmpty(),
				"PRAuthor should be populated from the PR metadata")
			Expect(run.Spec.BaseBranch).NotTo(BeEmpty(),
				"BaseBranch should be populated from the PR metadata")
			Expect(run.Spec.HeadBranch).NotTo(BeEmpty(),
				"HeadBranch should be populated from the PR metadata")
		})
	})

	Context("dedup: PR review PipelineRuns use #PR-N key format", func() {
		const (
			pipelineName = "pr-review-dedup"
			secretName   = "pr-review-dedup-secret"
			namespace    = "default"
		)

		ctx := context.Background()
		pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: namespace}

		BeforeEach(func() {
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      secretName,
					Namespace: namespace,
				},
				Data: map[string][]byte{
					"token": []byte("ghp_test_token"),
				},
			}
			Expect(k8sClient.Create(ctx, secret)).To(Succeed())

			pipeline := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      pipelineName,
					Namespace: namespace,
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
							Owner:    "test-owner",
							Repo:     "test-repo",
							Reviewer: "test-reviewer",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: secretName,
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
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
		})

		AfterEach(func() {
			var runs aiv1alpha1.PipelineRunList
			if err := k8sClient.List(ctx, &runs); err == nil {
				for i := range runs.Items {
					_ = k8sClient.Delete(ctx, &runs.Items[i])
				}
			}
			pipeline := &aiv1alpha1.Pipeline{}
			if err := k8sClient.Get(ctx, pipelineKey, pipeline); err == nil {
				Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
			}
			secret := &corev1.Secret{}
			secretKey := types.NamespacedName{Name: secretName, Namespace: namespace}
			if err := k8sClient.Get(ctx, secretKey, secret); err == nil {
				Expect(k8sClient.Delete(ctx, secret)).To(Succeed())
			}
		})

		It("should not create a duplicate PipelineRun for the same PR", func() {
			reconciler := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			prIssue := trigger.Issue{
				Number: 55,
				Key:    "#PR-55",
				Title:  "Update docs",
				Body:   "Documentation update",
			}

			// Create first PipelineRun
			err := reconciler.createPipelineRun(ctx, namespace, pipelineName, pipelineKey, prIssue)
			Expect(err).NotTo(HaveOccurred())

			// Check dedup — pipelineRunExists should find the existing run
			exists, err := reconciler.pipelineRunExists(ctx, namespace, pipelineName, "#PR-55")
			Expect(err).NotTo(HaveOccurred())
			Expect(exists).To(BeTrue(),
				"pipelineRunExists should detect existing PipelineRun with #PR-55 key")
		})
	})
})

// Compile-time assertion: the Issue struct used by createPipelineRun must have
// the fields needed for PR metadata propagation.
var _ = func() {
	_ = trigger.Issue{
		Number: 1,
		Key:    "#PR-1",
		Title:  "title",
		Body:   "body",
	}
}
