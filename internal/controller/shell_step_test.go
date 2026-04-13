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

var _ = Describe("shell step env and secret mounts", func() {
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
				Name:      "test-shell-step-run",
				Namespace: "default",
			},
			Spec: aiv1alpha1.PipelineRunSpec{
				PipelineRef: "test-shell-pipeline",
			},
			Status: aiv1alpha1.PipelineRunStatus{
				PVCName: "test-shell-step-run-workspace",
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

	Context("baseline behavior (no env or secrets)", func() {
		It("should create a shell container with workspace mount only", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "build",
				Type:     "shell",
				Commands: []string{"make build"},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			Expect(job.Spec.Template.Spec.Containers).To(HaveLen(1))
			c := job.Spec.Template.Spec.Containers[0]
			Expect(c.Image).To(Equal(shellImage))
			Expect(c.Env).To(BeEmpty())
			Expect(c.VolumeMounts).To(HaveLen(1))
			Expect(c.VolumeMounts[0].Name).To(Equal("workspace"))
		})
	})

	Context("per-step env vars", func() {
		It("should set plain env vars on the container", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "build",
				Type:     "shell",
				Commands: []string{"make build"},
				Env: []corev1.EnvVar{
					{Name: "GOOS", Value: "linux"},
					{Name: "GOARCH", Value: "amd64"},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			c := job.Spec.Template.Spec.Containers[0]
			Expect(c.Env).To(HaveLen(2))
			Expect(c.Env[0].Name).To(Equal("GOOS"))
			Expect(c.Env[0].Value).To(Equal("linux"))
			Expect(c.Env[1].Name).To(Equal("GOARCH"))
			Expect(c.Env[1].Value).To(Equal("amd64"))
		})

		It("should support valueFrom.secretKeyRef", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "deploy",
				Type:     "shell",
				Commands: []string{"./deploy.sh"},
				Env: []corev1.EnvVar{
					{
						Name: "REGISTRY_TOKEN",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "registry-creds"},
								Key:                  "token",
							},
						},
					},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			c := job.Spec.Template.Spec.Containers[0]
			Expect(c.Env).To(HaveLen(1))
			Expect(c.Env[0].Name).To(Equal("REGISTRY_TOKEN"))
			Expect(c.Env[0].ValueFrom).NotTo(BeNil())
			Expect(c.Env[0].ValueFrom.SecretKeyRef.Name).To(Equal("registry-creds"))
			Expect(c.Env[0].ValueFrom.SecretKeyRef.Key).To(Equal("token"))
		})
	})

	Context("secret mounts", func() {
		It("should mount a whole secret as a directory", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "build",
				Type:     "shell",
				Commands: []string{"make build"},
				SecretMounts: []aiv1alpha1.SecretMount{
					{SecretName: "tls-certs", MountPath: "/etc/ssl/custom"},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			// Verify volume was added
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(2)) // workspace + secret
			secretVol := job.Spec.Template.Spec.Volumes[1]
			Expect(secretVol.Name).To(Equal("secret-mount-0"))
			Expect(secretVol.Secret.SecretName).To(Equal("tls-certs"))

			// Verify volume mount
			c := job.Spec.Template.Spec.Containers[0]
			Expect(c.VolumeMounts).To(HaveLen(2))
			sm := c.VolumeMounts[1]
			Expect(sm.Name).To(Equal("secret-mount-0"))
			Expect(sm.MountPath).To(Equal("/etc/ssl/custom"))
			Expect(sm.ReadOnly).To(BeTrue())
			Expect(sm.SubPath).To(BeEmpty())
		})

		It("should mount a single key with subPath", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "build",
				Type:     "shell",
				Commands: []string{"npm install"},
				SecretMounts: []aiv1alpha1.SecretMount{
					{SecretName: "npm-auth", MountPath: "/root/.npmrc", Key: ".npmrc"},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			c := job.Spec.Template.Spec.Containers[0]
			sm := c.VolumeMounts[1]
			Expect(sm.MountPath).To(Equal("/root/.npmrc"))
			Expect(sm.SubPath).To(Equal(".npmrc"))
			Expect(sm.ReadOnly).To(BeTrue())
		})

		It("should handle multiple secret mounts with unique volume names", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "deploy",
				Type:     "shell",
				Commands: []string{"./deploy.sh"},
				SecretMounts: []aiv1alpha1.SecretMount{
					{SecretName: "tls-certs", MountPath: "/etc/ssl/custom"},
					{SecretName: "kubeconfig", MountPath: "/root/.kube/config", Key: "config"},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			// 3 volumes: workspace + 2 secrets
			Expect(job.Spec.Template.Spec.Volumes).To(HaveLen(3))
			Expect(job.Spec.Template.Spec.Volumes[1].Name).To(Equal("secret-mount-0"))
			Expect(job.Spec.Template.Spec.Volumes[2].Name).To(Equal("secret-mount-1"))

			c := job.Spec.Template.Spec.Containers[0]
			Expect(c.VolumeMounts).To(HaveLen(3))
			Expect(c.VolumeMounts[1].MountPath).To(Equal("/etc/ssl/custom"))
			Expect(c.VolumeMounts[1].SubPath).To(BeEmpty())
			Expect(c.VolumeMounts[2].MountPath).To(Equal("/root/.kube/config"))
			Expect(c.VolumeMounts[2].SubPath).To(Equal("config"))
		})
	})

	Context("env and secrets combined", func() {
		It("should wire both env vars and secret mounts", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "deploy",
				Type:     "shell",
				Commands: []string{"./deploy.sh"},
				Env: []corev1.EnvVar{
					{Name: "ENVIRONMENT", Value: "staging"},
				},
				SecretMounts: []aiv1alpha1.SecretMount{
					{SecretName: "deploy-key", MountPath: "/root/.ssh/id_rsa", Key: "id_rsa"},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)

			c := job.Spec.Template.Spec.Containers[0]
			Expect(c.Env).To(HaveLen(1))
			Expect(c.Env[0].Name).To(Equal("ENVIRONMENT"))
			Expect(c.VolumeMounts).To(HaveLen(2))
			Expect(c.VolumeMounts[1].Name).To(Equal("secret-mount-0"))
		})
	})

	Context("DinD interaction", func() {
		It("should preserve env vars when DinD sidecar is added", func() {
			step := &aiv1alpha1.StepSpec{
				Name:     "test",
				Type:     "shell",
				Commands: []string{"make test"},
				DinD:     true,
				Env: []corev1.EnvVar{
					{Name: "TEST_DB_HOST", Value: "localhost"},
				},
			}

			job := buildTestJob(run, step)
			reconciler.configureShellJob(job, step)
			addDinDSidecar(job)

			c := job.Spec.Template.Spec.Containers[0]
			// Should have both our env var and DOCKER_HOST from DinD
			envNames := make([]string, len(c.Env))
			for i, e := range c.Env {
				envNames[i] = e.Name
			}
			Expect(envNames).To(ContainElement("TEST_DB_HOST"))
			Expect(envNames).To(ContainElement("DOCKER_HOST"))
		})
	})
})
