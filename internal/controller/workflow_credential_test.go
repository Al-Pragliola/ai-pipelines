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
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("Workflow credential handling", func() {
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
				Name:      "test-wf-cred-run",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineRunSpec{
				PipelineRef: "test-pipeline",
				IssueNumber: 42,
				IssueTitle:  "Fix login bug",
				IssueBody:   "The login button is broken",
			},
			Status: aiv1alpha1.PipelineRunStatus{
				PVCName: "test-wf-cred-run-workspace",
				Branch:  "ai/42",
			},
		}
		Expect(k8sClient.Create(context.Background(), run)).To(Succeed())

		pipeline = &aiv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pipeline-wf-cred",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineSpec{
				Repo: &aiv1alpha1.RepoSpec{
					Owner: "test-org",
					Name:  "test-repo",
					SecretRef: aiv1alpha1.SecretKeyRef{
						Name: "pipeline-repo-token",
						Key:  "token",
					},
				},
				AI: aiv1alpha1.AISpec{
					Image: "ai-image:latest",
				},
				Steps: []aiv1alpha1.StepSpec{
					{
						Name:           "ai-step",
						Type:           "ai",
						PromptTemplate: "Fix issue {{.IssueNumber}}",
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

	findWorkflowInitContainer := func(job *batchv1.Job) *corev1.Container {
		for i := range job.Spec.Template.Spec.InitContainers {
			c := &job.Spec.Template.Spec.InitContainers[i]
			if c.Name == "workflow-clone" || strings.Contains(c.Name, "workflow") {
				return c
			}
		}
		return nil
	}

	findTokenEnv := func(container *corev1.Container) *corev1.EnvVar {
		for i := range container.Env {
			if container.Env[i].Name == "GITHUB_TOKEN" {
				return &container.Env[i]
			}
		}
		return nil
	}

	Context("WorkflowRef SecretRef field in CRD", func() {
		It("should have an optional SecretRef field on WorkflowRef", func() {
			ref := aiv1alpha1.WorkflowRef{
				Repo: "private-org/workflows",
				Path: "bugfix",
				Ref:  "main",
				SecretRef: &aiv1alpha1.SecretKeyRef{
					Name: "workflow-token",
					Key:  "token",
				},
			}
			Expect(ref.SecretRef).NotTo(BeNil())
			Expect(ref.SecretRef.Name).To(Equal("workflow-token"))
			Expect(ref.SecretRef.Key).To(Equal("token"))
		})

		It("should allow WorkflowRef without SecretRef (backward compatible)", func() {
			ref := aiv1alpha1.WorkflowRef{
				Repo: "public-org/workflows",
				Path: "bugfix",
			}
			Expect(ref.SecretRef).To(BeNil())
		})
	})

	Context("WorkflowRef SecretRef serialization via K8s API", func() {
		const resourceName = "test-wfcred-serial"

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

		It("should persist WorkflowRef.SecretRef through the API round-trip", func() {
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
								Repo: "private-org/workflows",
								Path: "bugfix",
								Ref:  "main",
								SecretRef: &aiv1alpha1.SecretKeyRef{
									Name: "workflow-specific-token",
									Key:  "gh-token",
								},
							},
							PromptTemplate: "Fix the bug",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			wfRef := fetched.Spec.Steps[0].WorkflowRef
			Expect(wfRef).NotTo(BeNil())
			Expect(wfRef.SecretRef).NotTo(BeNil(), "expected WorkflowRef.SecretRef to be persisted")
			Expect(wfRef.SecretRef.Name).To(Equal("workflow-specific-token"))
			Expect(wfRef.SecretRef.Key).To(Equal("gh-token"))
		})

		It("should persist WorkflowRef without SecretRef (nil) through the API round-trip", func() {
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
								Repo: "public-org/workflows",
								Path: "bugfix",
								Ref:  "main",
							},
							PromptTemplate: "Fix the bug",
						},
					},
				},
			}
			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			wfRef := fetched.Spec.Steps[0].WorkflowRef
			Expect(wfRef).NotTo(BeNil())
			Expect(wfRef.SecretRef).To(BeNil(), "expected WorkflowRef.SecretRef to be nil when not set")

			Expect(k8sClient.Delete(ctx, &fetched)).To(Succeed())
			Eventually(func() bool {
				err := k8sClient.Get(ctx, typeNamespacedName, &aiv1alpha1.Pipeline{})
				return errors.IsNotFound(err)
			}).Should(BeTrue())
		})
	})

	Context("When WorkflowRef has its own SecretRef", func() {
		It("should use the WorkflowRef SecretRef for the workflow clone GITHUB_TOKEN", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-wf-own-secret",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "private-org/workflows",
					Path: "bugfix",
					Ref:  "main",
					SecretRef: &aiv1alpha1.SecretKeyRef{
						Name: "workflow-repo-token",
						Key:  "wf-token",
					},
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			wfInit := findWorkflowInitContainer(job)
			Expect(wfInit).NotTo(BeNil(), "expected workflow-clone init container")

			tokenEnv := findTokenEnv(wfInit)
			Expect(tokenEnv).NotTo(BeNil(), "expected GITHUB_TOKEN env var")
			Expect(tokenEnv.ValueFrom).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("workflow-repo-token"),
				"expected workflow clone to use WorkflowRef.SecretRef, not pipeline repo secret")
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal("wf-token"),
				"expected workflow clone to use WorkflowRef.SecretRef key")
		})

		It("should use WorkflowRef SecretRef even when pipeline repo has a different secret", func() {
			pipeline.Spec.Repo.SecretRef = aiv1alpha1.SecretKeyRef{
				Name: "pipeline-main-secret",
				Key:  "main-token",
			}

			step := &aiv1alpha1.StepSpec{
				Name: "ai-wf-override",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "other-org/private-workflows",
					Path: "feature",
					Ref:  "v2",
					SecretRef: &aiv1alpha1.SecretKeyRef{
						Name: "other-org-token",
						Key:  "access-token",
					},
				},
				PromptTemplate: "Implement feature {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			wfInit := findWorkflowInitContainer(job)
			Expect(wfInit).NotTo(BeNil())

			tokenEnv := findTokenEnv(wfInit)
			Expect(tokenEnv).NotTo(BeNil())
			// Must use the WorkflowRef-specific secret, NOT the pipeline repo secret
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("other-org-token"),
				"WorkflowRef.SecretRef should take priority over pipeline.Spec.Repo.SecretRef")
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal("access-token"))
		})
	})

	Context("When WorkflowRef does NOT have its own SecretRef", func() {
		It("should fall back to pipeline repo secretRef for the workflow clone", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-wf-fallback",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "same-org/workflows",
					Path: "bugfix",
					Ref:  "main",
					// No SecretRef — should fall back to pipeline.Spec.Repo.SecretRef
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			wfInit := findWorkflowInitContainer(job)
			Expect(wfInit).NotTo(BeNil())

			tokenEnv := findTokenEnv(wfInit)
			Expect(tokenEnv).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("pipeline-repo-token"),
				"should fall back to pipeline repo secret when WorkflowRef.SecretRef is nil")
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal("token"))
		})
	})

	Context("When pipeline has a trigger secretRef and no repo", func() {
		It("should use the trigger's GitHub secretRef when no repo secretRef and no WorkflowRef secretRef", func() {
			pipelineNoRepo := &aiv1alpha1.Pipeline{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-pipeline-trigger-cred",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineSpec{
					Trigger: &aiv1alpha1.TriggerSpec{
						GitHub: &aiv1alpha1.GitHubTriggerSpec{
							Owner:    "trigger-org",
							Repo:     "trigger-repo",
							Assignee: "bot",
							SecretRef: aiv1alpha1.SecretKeyRef{
								Name: "trigger-github-token",
								Key:  "token",
							},
						},
					},
					AI: aiv1alpha1.AISpec{
						Image: "ai-image:latest",
					},
					Steps: []aiv1alpha1.StepSpec{
						{
							Name:           "ai-step",
							Type:           "ai",
							PromptTemplate: "Fix issue",
						},
					},
				},
			}

			step := &aiv1alpha1.StepSpec{
				Name: "ai-trigger-token",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "same-org/workflows",
					Path: "bugfix",
					Ref:  "main",
					// No SecretRef and no pipeline.Spec.Repo — should use trigger token
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipelineNoRepo.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipelineNoRepo, step)
			Expect(err).NotTo(HaveOccurred())

			wfInit := findWorkflowInitContainer(job)
			Expect(wfInit).NotTo(BeNil(), "expected workflow-clone init container")

			tokenEnv := findTokenEnv(wfInit)
			Expect(tokenEnv).NotTo(BeNil(), "expected GITHUB_TOKEN env var")
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("trigger-github-token"),
				"should use the trigger's secretRef when no repo or WorkflowRef secretRef is available")
		})
	})

	Context("WorkflowRef SecretRef with default key", func() {
		It("should default the key to 'token' when only name is specified", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-wf-default-key",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "private-org/workflows",
					Path: "bugfix",
					Ref:  "main",
					SecretRef: &aiv1alpha1.SecretKeyRef{
						Name: "workflow-secret",
						// Key omitted — should default to "token"
					},
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			wfInit := findWorkflowInitContainer(job)
			Expect(wfInit).NotTo(BeNil())

			tokenEnv := findTokenEnv(wfInit)
			Expect(tokenEnv).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("workflow-secret"))
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal("token"),
				"should default to 'token' key when Key is not specified")
		})
	})
})
