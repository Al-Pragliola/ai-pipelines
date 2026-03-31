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
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("watch-report step type", func() {
	var (
		reconciler *PipelineRunReconciler
		run        *aiv1alpha1.PipelineRun
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
				Name:      "test-watch-report-run",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineRunSpec{
				PipelineRef: "test-watch-report-pipeline",
			},
			Status: aiv1alpha1.PipelineRunStatus{
				PVCName: "test-watch-report-run-workspace",
			},
		}
		Expect(k8sClient.Create(context.Background(), run)).To(Succeed())
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

	Context("When configuring a watch-report Job", func() {
		It("should create a container that cats the report file", func() {
			step := &aiv1alpha1.StepSpec{
				Name:       "show-review",
				Type:       "watch-report",
				ReportFile: "/workspace/review.md",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureWatchReportJob(job, step)
			Expect(err).NotTo(HaveOccurred())

			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			container := job.Spec.Template.Spec.Containers[0]

			Expect(container.Name).To(Equal("report"))
			Expect(container.Image).To(Equal(readerImage))
			Expect(container.Command).To(Equal([]string{"/bin/sh", "-c"}))
			Expect(container.Args).To(Equal([]string{"cat /workspace/review.md"}))
		})

		It("should mount the workspace volume", func() {
			step := &aiv1alpha1.StepSpec{
				Name:       "show-review",
				Type:       "watch-report",
				ReportFile: "/workspace/review.md",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureWatchReportJob(job, step)
			Expect(err).NotTo(HaveOccurred())

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.VolumeMounts).To(HaveLen(1))
			Expect(container.VolumeMounts[0].Name).To(Equal("workspace"))
			Expect(container.VolumeMounts[0].MountPath).To(Equal(workspacePath))
		})

		It("should fail when reportFile is not set", func() {
			step := &aiv1alpha1.StepSpec{
				Name: "show-review",
				Type: "watch-report",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureWatchReportJob(job, step)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("requires reportFile"))
		})

		It("should work with custom report file paths", func() {
			step := &aiv1alpha1.StepSpec{
				Name:       "show-analysis",
				Type:       "watch-report",
				ReportFile: "/workspace/output/analysis.md",
			}

			job := buildTestJob(run, step)
			err := reconciler.configureWatchReportJob(job, step)
			Expect(err).NotTo(HaveOccurred())

			container := job.Spec.Template.Spec.Containers[0]
			Expect(container.Args).To(Equal([]string{"cat /workspace/output/analysis.md"}))
		})
	})
})
