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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("git-checkout-pr step type", func() {
	var (
		reconciler *PipelineRunReconciler
		run        *aiv1alpha1.PipelineRun
		pipeline   *aiv1alpha1.Pipeline
		scheme     *runtime.Scheme
	)

	BeforeEach(func() {
		scheme = k8sClient.Scheme()
		reconciler = &PipelineRunReconciler{
			Client: k8sClient,
			Scheme: scheme,
		}

		run = &aiv1alpha1.PipelineRun{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-checkout-pr-run",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineRunSpec{
				PipelineRef: "test-pr-review-pipeline",
				PRNumber:    123,
				PRTitle:     "Add new feature",
				PRBody:      "This PR adds a new feature",
				PRAuthor:    "contributor",
				BaseBranch:  "main",
				HeadBranch:  "feature-branch",
			},
			Status: aiv1alpha1.PipelineRunStatus{
				PVCName: "test-checkout-pr-run-workspace",
			},
		}
		Expect(k8sClient.Create(context.Background(), run)).To(Succeed())

		pipeline = &aiv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pr-review-pipeline",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineSpec{
				Trigger: &aiv1alpha1.TriggerSpec{
					GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
						Owner:    "upstream-org",
						Repo:     "upstream-repo",
						Reviewer: "review-bot",
						SecretRef: aiv1alpha1.SecretKeyRef{
							Name: "pr-review-token",
						},
					},
				},
				AI: aiv1alpha1.AISpec{
					Image: "ai-image:latest",
				},
				Steps: []aiv1alpha1.StepSpec{
					{
						Name: "checkout-pr",
						Type: "git-checkout-pr",
					},
					{
						Name:           "review",
						Type:           "ai",
						PromptTemplate: "Review PR {{.PRNumber}}",
					},
				},
			},
		}
	})

	AfterEach(func() {
		_ = k8sClient.Delete(context.Background(), run)
	})

	buildTestJob := func(run *aiv1alpha1.PipelineRun, step *aiv1alpha1.StepSpec) *batchv1.Job {
		var backoffLimit int32 = 0
		job := &batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{
				Name:      run.Name + "-" + step.Name + "-1",
				Namespace: run.Namespace,
			},
			Spec: batchv1.JobSpec{
				BackoffLimit: &backoffLimit,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Volumes: []corev1.Volume{
							{
								Name: "workspace",
								VolumeSource: corev1.VolumeSource{
									PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
										ClaimName: run.Status.PVCName,
									},
								},
							},
						},
					},
				},
			},
		}
		Expect(controllerutil.SetControllerReference(run, job, scheme)).To(Succeed())
		return job
	}

	Context("When configuring a git-checkout-pr Job", func() {
		It("should create a container that clones the trigger repo and fetches the PR head ref", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Should have exactly one container
			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]

			// Container name should identify this as a checkout-pr step
			Expect(container.Name).To(Equal("checkout-pr"))

			// Should use the alpine/git image
			Expect(container.Image).To(Equal(gitImage))

			// Should use /bin/sh -c
			Expect(container.Command).To(Equal([]string{"/bin/sh", "-c"}))
		})

		It("should clone the upstream repo from the githubPRReview trigger config", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			// Should clone owner/repo from the trigger config
			Expect(script).To(ContainSubstring("upstream-org/upstream-repo"),
				"should clone the repo specified in the githubPRReview trigger")
			Expect(script).To(ContainSubstring("git clone"),
				"should use git clone")
		})

		It("should fetch refs/pull/<PRNumber>/head", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			// Should fetch the PR head via the special GitHub ref
			Expect(script).To(ContainSubstring("refs/pull/123/head"),
				"should fetch the PR head using refs/pull/<PRNumber>/head")
			Expect(script).To(ContainSubstring("git fetch"),
				"should use git fetch to retrieve the PR ref")
		})

		It("should checkout FETCH_HEAD after fetching the PR ref", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			// Should checkout FETCH_HEAD to get the PR's actual code
			Expect(script).To(ContainSubstring("git checkout FETCH_HEAD"),
				"should checkout FETCH_HEAD after fetching the PR ref")
		})

		It("should strip credentials from the remote URL after checkout", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			// Should strip credentials by resetting the remote URL
			Expect(script).To(ContainSubstring("git remote set-url origin"),
				"should strip credentials from origin remote")
			// The clean URL should not contain x-access-token
			// Find the set-url line and check it uses a clean URL
			Expect(script).To(ContainSubstring("https://github.com/upstream-org/upstream-repo.git"),
				"should set remote to credential-free URL")
		})

		It("should set ownership to 1000:1000", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			Expect(script).To(ContainSubstring("chown -R 1000:1000"),
				"should set workspace ownership to 1000:1000")
		})

		It("should mount the workspace volume", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			container := job.Spec.Template.Spec.Containers[0]
			var mountsWorkspace bool
			for _, vm := range container.VolumeMounts {
				if vm.Name == "workspace" && vm.MountPath == workspacePath {
					mountsWorkspace = true
					break
				}
			}
			Expect(mountsWorkspace).To(BeTrue(),
				"should mount the workspace volume at "+workspacePath)
		})

		It("should inject GITHUB_TOKEN from the trigger's secretRef", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			container := job.Spec.Template.Spec.Containers[0]
			var tokenEnv *corev1.EnvVar
			for i := range container.Env {
				if container.Env[i].Name == githubTokenEnvVar {
					tokenEnv = &container.Env[i]
					break
				}
			}
			Expect(tokenEnv).NotTo(BeNil(), "should have a GITHUB_TOKEN env var")
			Expect(tokenEnv.ValueFrom).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("pr-review-token"),
				"should reference the trigger's secretRef")
		})

		It("should use a custom secret key when specified in secretRef", func() {
			pipeline.Spec.Trigger.GitHubPRReview.SecretRef = aiv1alpha1.SecretKeyRef{
				Name: "custom-secret",
				Key:  "gh-pat",
			}

			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			container := job.Spec.Template.Spec.Containers[0]
			var tokenEnv *corev1.EnvVar
			for i := range container.Env {
				if container.Env[i].Name == githubTokenEnvVar {
					tokenEnv = &container.Env[i]
					break
				}
			}
			Expect(tokenEnv).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("custom-secret"))
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal("gh-pat"))
		})

		It("should NOT create a new branch (no git checkout -b)", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			Expect(script).NotTo(ContainSubstring("checkout -b"),
				"should NOT create a new branch — only check out the existing PR head")
		})

		It("should NOT add a fork remote", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			Expect(script).NotTo(ContainSubstring("remote add fork"),
				"should NOT add a fork remote — this is a read-only checkout")
		})

		It("should use set -e for fail-fast behavior", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			script := strings.Join(job.Spec.Template.Spec.Containers[0].Args, " ")

			Expect(script).To(ContainSubstring("set -e"),
				"script should use set -e for fail-fast")
		})
	})

	Context("When the pipeline has no githubPRReview trigger", func() {
		It("should return an error", func() {
			pipeline.Spec.Trigger = nil

			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).To(HaveOccurred(),
				"should fail when pipeline has no trigger")
		})

		It("should return an error when trigger exists but githubPRReview is nil", func() {
			pipeline.Spec.Trigger = &aiv1alpha1.TriggerSpec{
				// No GitHubPRReview set
			}

			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).To(HaveOccurred(),
				"should fail when githubPRReview trigger is not configured")
		})
	})

	Context("When PRNumber is 0 (not set)", func() {
		It("should return an error", func() {
			run.Spec.PRNumber = 0

			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureCheckoutPRJob(context.Background(), job, run, pipeline, step)
			Expect(err).To(HaveOccurred(),
				"should fail when PRNumber is 0")
		})
	})

	Context("When the step type is dispatched via buildJob", func() {
		It("should be handled in the step dispatch switch", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "checkout-pr",
				Type: "git-checkout-pr",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job, err := reconciler.buildJob(context.Background(), run, pipeline, step, 1)
			Expect(err).NotTo(HaveOccurred())
			Expect(job).NotTo(BeNil())

			// The job should have been configured with a container (not left empty)
			Expect(job.Spec.Template.Spec.Containers).NotTo(BeEmpty(),
				"buildJob should dispatch git-checkout-pr to configureCheckoutPRJob")

			// Verify it's the checkout-pr container, not a default/empty one
			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Name).To(Equal("checkout-pr"))
			Expect(container.Image).To(Equal(gitImage))
		})
	})
})
