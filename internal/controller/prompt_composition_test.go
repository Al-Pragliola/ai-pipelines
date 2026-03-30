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
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("Prompt composition with workflowRef", func() {
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
				Name:      "test-prompt-comp-run",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineRunSpec{
				PipelineRef: "test-pipeline",
				IssueNumber: 99,
				IssueKey:    "#99",
				IssueTitle:  "Login button broken on mobile",
				IssueBody:   "When tapping the login button on iOS Safari, nothing happens.",
				Description: "Fix the login button on mobile Safari",
			},
			Status: aiv1alpha1.PipelineRunStatus{
				PVCName: "test-prompt-comp-run-workspace",
				Branch:  "ai/99",
			},
		}
		Expect(k8sClient.Create(context.Background(), run)).To(Succeed())

		pipeline = &aiv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-pipeline-prompt-comp",
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
		// Clean up ConfigMaps created by configureAIJob
		cmList := &corev1.ConfigMapList{}
		_ = k8sClient.List(context.Background(), cmList)
		for i := range cmList.Items {
			if strings.HasPrefix(cmList.Items[i].Name, "test-prompt-comp-run") {
				_ = k8sClient.Delete(context.Background(), &cmList.Items[i])
			}
		}
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

	// Helper to extract prompt from the ConfigMap created by configureAIJob
	getPromptFromConfigMap := func(jobName string) string {
		cmName := jobName + "-prompt"
		var cm corev1.ConfigMap
		err := k8sClient.Get(context.Background(), types.NamespacedName{
			Name:      cmName,
			Namespace: "default",
		}, &cm)
		Expect(err).NotTo(HaveOccurred(), "expected prompt ConfigMap %q to exist", cmName)
		prompt, ok := cm.Data["prompt.txt"]
		Expect(ok).To(BeTrue(), "expected prompt.txt key in ConfigMap")
		return prompt
	}

	Context("When workflowRef is set and promptTemplate is provided", func() {
		It("should use the rendered promptTemplate as the user message (contextual prompt)", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-with-ctx-prompt",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Run the full bugfix workflow for issue #{{.IssueNumber}}: {{.IssueTitle}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			prompt := getPromptFromConfigMap(job.Name)

			// The prompt should contain the rendered issue context
			Expect(prompt).To(ContainSubstring("99"), "expected issue number in prompt")
			Expect(prompt).To(ContainSubstring("Login button broken on mobile"), "expected issue title in prompt")

			// When workflowRef is set, the prompt should NOT contain methodology/system-level
			// instructions — those come from the workflow's .ambient/ directory.
			// The prompt should be a short contextual message (the user message).
			// It should contain the rendered template text.
			Expect(prompt).To(ContainSubstring("Run the full bugfix workflow for issue #99"))
		})

		It("should store the prompt in a ConfigMap and pipe it to claude as the user message", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-prompt-piped",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "Fix the bug in issue #{{.IssueNumber}}",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			// The AI container should pipe the prompt file to claude
			Expect(job.Spec.Template.Spec.Containers).NotTo(BeEmpty())
			aiContainer := job.Spec.Template.Spec.Containers[0]
			Expect(aiContainer.Name).To(Equal("ai"))

			// The script should pipe the prompt to claude
			script := strings.Join(aiContainer.Args, " ")
			Expect(script).To(ContainSubstring("prompt.txt"))
			Expect(script).To(ContainSubstring("claude"))
		})
	})

	Context("When workflowRef is set and promptTemplate is empty", func() {
		It("should auto-generate a prompt from issue fields", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-auto-prompt",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "", // empty — should auto-generate
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			prompt := getPromptFromConfigMap(job.Name)

			// The auto-generated prompt should include issue context
			Expect(prompt).NotTo(BeEmpty(), "expected auto-generated prompt when promptTemplate is empty and workflowRef is set")
			Expect(prompt).To(ContainSubstring("99"), "expected issue number in auto-generated prompt")
			Expect(prompt).To(ContainSubstring("Login button broken on mobile"), "expected issue title in auto-generated prompt")
			Expect(prompt).To(ContainSubstring("tapping the login button"), "expected issue body content in auto-generated prompt")
		})

		It("should auto-generate a prompt from description when no issue fields are set", func() {
			// Create a run without issue fields (spot run)
			spotRun := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-prompt-comp-spot",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					Description: "Refactor the authentication module to use JWT tokens",
				},
				Status: aiv1alpha1.PipelineRunStatus{
					PVCName: "test-prompt-comp-spot-workspace",
					Branch:  "ai/spot-1",
				},
			}
			Expect(k8sClient.Create(context.Background(), spotRun)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(context.Background(), spotRun)
			}()

			step := &aiv1alpha1.StepSpec{
				Name: "ai-auto-spot",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "refactor",
					Ref:  "main",
				},
				PromptTemplate: "", // empty
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(spotRun, step)
			err := reconciler.configureAIJob(context.Background(), job, spotRun, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			cmName := job.Name + "-prompt"
			var cm corev1.ConfigMap
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      cmName,
				Namespace: "default",
			}, &cm)
			Expect(err).NotTo(HaveOccurred())

			prompt := cm.Data["prompt.txt"]
			Expect(prompt).NotTo(BeEmpty(), "expected auto-generated prompt from description for spot run")
			Expect(prompt).To(ContainSubstring("Refactor the authentication module"), "expected description in auto-generated prompt")
		})
	})

	Context("When workflowRef is NOT set (backward compatibility)", func() {
		It("should use promptTemplate as-is for the full prompt (no auto-generation)", func() {
			step := &aiv1alpha1.StepSpec{
				Name:           "ai-no-workflow",
				Type:           "ai",
				PromptTemplate: "You are an expert developer. Fix issue #{{.IssueNumber}}: {{.IssueTitle}}\n\nDetails: {{.IssueBody}}\n\nFollow these steps:\n1. Read the code\n2. Write a fix\n3. Run tests",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			prompt := getPromptFromConfigMap(job.Name)

			// Without workflowRef, the prompt should be the full rendered template
			// including methodology instructions
			Expect(prompt).To(ContainSubstring("You are an expert developer"))
			Expect(prompt).To(ContainSubstring("Follow these steps"))
			Expect(prompt).To(ContainSubstring("99"))
		})

		It("should produce an empty prompt when promptTemplate is empty and no workflowRef", func() {
			step := &aiv1alpha1.StepSpec{
				Name:           "ai-empty-no-wf",
				Type:           "ai",
				PromptTemplate: "",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			prompt := getPromptFromConfigMap(job.Name)

			// Without workflowRef and empty promptTemplate, the prompt should be empty
			// (no auto-generation — that only happens with workflowRef)
			Expect(prompt).To(BeEmpty(), "expected empty prompt when no workflowRef and empty promptTemplate")
		})
	})

	Context("Auto-generated prompt content", func() {
		It("should include issue number, title, and body in the auto-generated prompt", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-auto-full",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			job := buildTestJob(run, step)
			err := reconciler.configureAIJob(context.Background(), job, run, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			prompt := getPromptFromConfigMap(job.Name)

			// All available issue fields should be present
			Expect(prompt).To(ContainSubstring("#99"), "expected issue key in auto-generated prompt")
			Expect(prompt).To(ContainSubstring("Login button broken on mobile"), "expected issue title")
			Expect(prompt).To(ContainSubstring("tapping the login button on iOS Safari"), "expected issue body")
		})

		It("should include the description field in the auto-generated prompt", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "ai-auto-desc",
				Type: "ai",
				WorkflowRef: &aiv1alpha1.WorkflowRef{
					Repo: "ambient-code/workflows",
					Path: "bugfix",
					Ref:  "main",
				},
				PromptTemplate: "",
			}
			pipeline.Spec.Steps = []aiv1alpha1.StepSpec{*step}

			// Use a run that has description but minimal issue fields
			descRun := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-prompt-comp-desc",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					IssueNumber: 55,
					IssueTitle:  "Add dark mode",
					Description: "Implement dark mode toggle in settings page",
				},
				Status: aiv1alpha1.PipelineRunStatus{
					PVCName: "test-prompt-comp-desc-workspace",
					Branch:  "ai/55",
				},
			}
			Expect(k8sClient.Create(context.Background(), descRun)).To(Succeed())
			defer func() {
				_ = k8sClient.Delete(context.Background(), descRun)
			}()

			job := buildTestJob(descRun, step)
			err := reconciler.configureAIJob(context.Background(), job, descRun, pipeline, step)
			Expect(err).NotTo(HaveOccurred())

			cmName := job.Name + "-prompt"
			var cm corev1.ConfigMap
			err = k8sClient.Get(context.Background(), types.NamespacedName{
				Name:      cmName,
				Namespace: "default",
			}, &cm)
			Expect(err).NotTo(HaveOccurred())

			prompt := cm.Data["prompt.txt"]
			Expect(prompt).To(SatisfyAny(
				ContainSubstring("Add dark mode"),
				ContainSubstring("Implement dark mode toggle"),
			), "expected issue title or description in auto-generated prompt")
		})
	})
})
