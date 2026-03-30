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

var _ = Describe("Workflow clone init container", func() {
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
				Name:      "test-wf-clone-run",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineRunSpec{
				PipelineRef: "test-pipeline",
				IssueNumber: 42,
				IssueTitle:  "Fix login bug",
				IssueBody:   "The login button is broken",
			},
			Status: aiv1alpha1.PipelineRunStatus{
				PVCName: "test-wf-clone-run-workspace",
				Branch:  "ai/42",
			},
		}
		Expect(k8sClient.Create(context.Background(), run)).To(Succeed())

		pipeline = &aiv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pipeline-wf-clone",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineSpec{
				Repo: &aiv1alpha1.RepoSpec{
					Owner: "test-org",
					Name:  "test-repo",
					SecretRef: aiv1alpha1.SecretKeyRef{
						Name: "github-token",
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
		// Clean up the PipelineRun
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

	Context("When an AI step has workflowRef", func() {
		It("should add a git clone init container before the AI container", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-with-workflow",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// There should be at least one init container for the workflow clone
			Expect(job.Spec.Template.Spec.InitContainers).NotTo(BeEmpty(),
				"expected init container for workflow clone but found none")

			// Find the workflow clone init container
			var wfInit *corev1.Container
			for i := range job.Spec.Template.Spec.InitContainers {
				c := &job.Spec.Template.Spec.InitContainers[i]
				if c.Name == "workflow-clone" || strings.Contains(c.Name, "workflow") {
					wfInit = c
					break
				}
			}
			Expect(wfInit).NotTo(BeNil(), "expected an init container named 'workflow-clone'")

			// It should use the alpine/git image (same as configureCheckoutJob)
			Expect(wfInit.Image).To(Equal(gitImage))

			// It should have a GITHUB_TOKEN env var from the secret
			var hasToken bool
			for _, env := range wfInit.Env {
				if env.Name == "GITHUB_TOKEN" && env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
					hasToken = true
					Expect(env.ValueFrom.SecretKeyRef.Name).To(Equal("github-token"))
					break
				}
			}
			Expect(hasToken).To(BeTrue(), "expected GITHUB_TOKEN env var from github-token secret")

			// It should mount the workspace volume
			var mountsWorkspace bool
			for _, vm := range wfInit.VolumeMounts {
				if vm.Name == "workspace" && vm.MountPath == workspacePath {
					mountsWorkspace = true
					break
				}
			}
			Expect(mountsWorkspace).To(BeTrue(), "expected workspace volume mount on init container")
		})

		It("should clone the workflow repo shallow and single-branch", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-shallow-clone",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Find the workflow clone init container
			var wfInit *corev1.Container
			for i := range job.Spec.Template.Spec.InitContainers {
				c := &job.Spec.Template.Spec.InitContainers[i]
				if strings.Contains(c.Name, "workflow") {
					wfInit = c
					break
				}
			}
			Expect(wfInit).NotTo(BeNil())

			// The script should contain shallow clone flags
			script := strings.Join(wfInit.Args, " ")
			Expect(script).To(ContainSubstring("--depth"), "expected shallow clone (--depth)")
			Expect(script).To(ContainSubstring("--single-branch"), "expected single-branch clone")
			Expect(script).To(ContainSubstring("ambient-code/workflows"), "expected workflow repo in clone URL")
		})

		It("should copy .claude/, .ambient/, and CLAUDE.md from {path}/ into workspace root", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-copy-files",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Find the workflow clone init container
			var wfInit *corev1.Container
			for i := range job.Spec.Template.Spec.InitContainers {
				c := &job.Spec.Template.Spec.InitContainers[i]
				if strings.Contains(c.Name, "workflow") {
					wfInit = c
					break
				}
			}
			Expect(wfInit).NotTo(BeNil())

			script := strings.Join(wfInit.Args, " ")
			// Should copy .claude/ directory
			Expect(script).To(ContainSubstring(".claude"), "expected .claude/ to be copied from workflow")
			// Should copy .ambient/ directory
			Expect(script).To(ContainSubstring(".ambient"), "expected .ambient/ to be copied from workflow")
			// Should copy CLAUDE.md
			Expect(script).To(ContainSubstring("CLAUDE.md"), "expected CLAUDE.md to be copied from workflow")
			// Should reference the path within the workflow repo
			Expect(script).To(ContainSubstring("bugfix"), "expected workflow path 'bugfix' in copy script")
		})

		It("should not overwrite existing workspace CLAUDE.md (workspace takes precedence)", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-merge-precedence",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Find the workflow clone init container
			var wfInit *corev1.Container
			for i := range job.Spec.Template.Spec.InitContainers {
				c := &job.Spec.Template.Spec.InitContainers[i]
				if strings.Contains(c.Name, "workflow") {
					wfInit = c
					break
				}
			}
			Expect(wfInit).NotTo(BeNil())

			script := strings.Join(wfInit.Args, " ")
			// The script should guard against overwriting existing CLAUDE.md
			// This can be done with a conditional copy (e.g., cp --no-clobber or [ ! -f ] check)
			// The exact mechanism depends on implementation, but the script should NOT
			// blindly overwrite CLAUDE.md
			Expect(script).To(SatisfyAny(
				ContainSubstring("--no-clobber"),
				ContainSubstring("-n"),
				ContainSubstring("! -f"),
				ContainSubstring("if ["),
			), "expected CLAUDE.md copy to preserve existing workspace file")
		})

		It("should place the init container before the AI main container", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-ordering",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Init containers run before main containers by definition
			Expect(job.Spec.Template.Spec.InitContainers).NotTo(BeEmpty(),
				"workflow clone should be an init container (runs before main)")
			Expect(job.Spec.Template.Spec.Containers).NotTo(BeEmpty(),
				"AI main container should still be present")

			// The main container should be named "ai"
			Expect(job.Spec.Template.Spec.Containers[0].Name).To(Equal("ai"))
		})

		It("should use a temp volume for cloning (not pollute workspace)", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-temp-vol",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Should have a temp/emptyDir volume for the workflow clone
			var hasTempVol bool
			for _, v := range job.Spec.Template.Spec.Volumes {
				if v.EmptyDir != nil && strings.Contains(v.Name, "workflow") {
					hasTempVol = true
					break
				}
			}
			Expect(hasTempVol).To(BeTrue(), "expected a temp emptyDir volume for workflow clone")
		})

		It("should handle workflowRef without ref (defaults to HEAD)", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-no-ref",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					// No Ref — should default to HEAD
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// Should still have the init container
			Expect(job.Spec.Template.Spec.InitContainers).NotTo(BeEmpty(),
				"expected init container even without explicit ref")
		})
	})

	Context("When an AI step does NOT have workflowRef", func() {
		It("should not add a workflow clone init container", func() {
			step := &aiv1alpha1.StepSpec{
				Name:           "ai-no-workflow",
				Type:           "ai",
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// No init containers should be present (no workflow to clone)
			for _, c := range job.Spec.Template.Spec.InitContainers {
				Expect(c.Name).NotTo(ContainSubstring("workflow"),
					"should not have a workflow init container when workflowRef is nil")
			}
		})
	})

	Context("When workflowRef uses the same secret as checkout", func() {
		It("should reference the pipeline repo secretRef for the GITHUB_TOKEN", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-secret-ref",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}
			pipeline.Spec.Repo.SecretRef = aiv1alpha1.SecretKeyRef{
				Name: "my-github-secret",
				Key:  "gh-token",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			var wfInit *corev1.Container
			for i := range job.Spec.Template.Spec.InitContainers {
				c := &job.Spec.Template.Spec.InitContainers[i]
				if strings.Contains(c.Name, "workflow") {
					wfInit = c
					break
				}
			}
			Expect(wfInit).NotTo(BeNil())

			var tokenEnv *corev1.EnvVar
			for i := range wfInit.Env {
				if wfInit.Env[i].Name == "GITHUB_TOKEN" {
					tokenEnv = &wfInit.Env[i]
					break
				}
			}
			Expect(tokenEnv).NotTo(BeNil())
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Name).To(Equal("my-github-secret"))
			Expect(tokenEnv.ValueFrom.SecretKeyRef.Key).To(Equal("gh-token"))
		})
	})

	Context("Merge behavior for .claude/ content", func() {
		It("should merge workflow .claude/ with existing workspace .claude/ content", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-merge-claude",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix issue {{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			var wfInit *corev1.Container
			for i := range job.Spec.Template.Spec.InitContainers {
				c := &job.Spec.Template.Spec.InitContainers[i]
				if strings.Contains(c.Name, "workflow") {
					wfInit = c
					break
				}
			}
			Expect(wfInit).NotTo(BeNil())

			script := strings.Join(wfInit.Args, " ")
			// The copy should merge (not replace) .claude/ directory contents
			// This means using cp with flags that don't delete existing files
			// (e.g., cp -rn or rsync-like behavior)
			Expect(script).To(SatisfyAny(
				ContainSubstring("-rn"),
				ContainSubstring("--no-clobber"),
				ContainSubstring("cp -r"),
				ContainSubstring("rsync"),
			), "expected merge behavior when copying .claude/ directory")
		})
	})
})
