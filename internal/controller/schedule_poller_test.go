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

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("Schedule Poller", func() {
	const pipelineName = "test-sched-poller"
	const namespace = "default"

	ctx := context.Background()

	BeforeEach(func() {
		pipeline := &aiv1alpha1.Pipeline{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pipelineName,
				Namespace: namespace,
			},
			Spec: aiv1alpha1.PipelineSpec{
				Trigger: &aiv1alpha1.TriggerSpec{
					Schedule: &aiv1alpha1.ScheduleTriggerSpec{
						Schedule: "0 0 * * 0",
						Prompt:   "test prompt",
					},
				},
				AI: aiv1alpha1.AISpec{
					Image: "test:latest",
				},
				Steps: []aiv1alpha1.StepSpec{
					{Name: "checkout", Type: "git-checkout"},
				},
			},
		}
		Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())
	})

	AfterEach(func() {
		// Clean up PipelineRuns
		var runs aiv1alpha1.PipelineRunList
		Expect(k8sClient.List(ctx, &runs)).To(Succeed())
		for i := range runs.Items {
			Expect(k8sClient.Delete(ctx, &runs.Items[i])).To(Succeed())
		}

		// Clean up Pipeline
		pipeline := &aiv1alpha1.Pipeline{}
		if err := k8sClient.Get(ctx, types.NamespacedName{Name: pipelineName, Namespace: namespace}, pipeline); err == nil {
			Expect(k8sClient.Delete(ctx, pipeline)).To(Succeed())
		}
	})

	Describe("hasActiveScheduledRun", func() {
		It("should return false when no runs exist", func() {
			r := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			active, err := r.hasActiveScheduledRun(ctx, namespace, pipelineName)
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(BeFalse())
		})

		It("should return true when a non-terminal run exists", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: pipelineName + "-sched-",
					Namespace:    namespace,
					Labels: map[string]string{
						"ai.aipipelines.io/pipeline":     pipelineName,
						"ai.aipipelines.io/trigger-type": "schedule",
					},
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: pipelineName,
					Description: "test prompt",
				},
			}
			Expect(k8sClient.Create(ctx, run)).To(Succeed())

			r := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			active, err := r.hasActiveScheduledRun(ctx, namespace, pipelineName)
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(BeTrue())
		})

		It("should return false when all runs are terminal", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: pipelineName + "-sched-",
					Namespace:    namespace,
					Labels: map[string]string{
						"ai.aipipelines.io/pipeline":     pipelineName,
						"ai.aipipelines.io/trigger-type": "schedule",
					},
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: pipelineName,
					Description: "test prompt",
				},
			}
			Expect(k8sClient.Create(ctx, run)).To(Succeed())

			// Update status to terminal
			run.Status.Phase = aiv1alpha1.PipelineRunPhaseSucceeded
			Expect(k8sClient.Status().Update(ctx, run)).To(Succeed())

			r := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			active, err := r.hasActiveScheduledRun(ctx, namespace, pipelineName)
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(BeFalse())
		})

		It("should ignore runs from other pipelines", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					GenerateName: "other-pipeline-sched-",
					Namespace:    namespace,
					Labels: map[string]string{
						"ai.aipipelines.io/pipeline":     "other-pipeline",
						"ai.aipipelines.io/trigger-type": "schedule",
					},
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "other-pipeline",
					Description: "other prompt",
				},
			}
			Expect(k8sClient.Create(ctx, run)).To(Succeed())

			r := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}
			active, err := r.hasActiveScheduledRun(ctx, namespace, pipelineName)
			Expect(err).NotTo(HaveOccurred())
			Expect(active).To(BeFalse())
		})
	})

	Describe("createScheduledPipelineRun", func() {
		It("should create a PipelineRun with correct labels and description", func() {
			r := &PipelineReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			pipelineKey := types.NamespacedName{Name: pipelineName, Namespace: namespace}
			prompt := "Run weekly security scan"
			err := r.createScheduledPipelineRun(ctx, namespace, pipelineName, pipelineKey, prompt)
			Expect(err).NotTo(HaveOccurred())

			// Verify the run was created with correct attributes
			var runs aiv1alpha1.PipelineRunList
			Expect(k8sClient.List(ctx, &runs)).To(Succeed())

			var found *aiv1alpha1.PipelineRun
			for i := range runs.Items {
				if runs.Items[i].Labels["ai.aipipelines.io/trigger-type"] == "schedule" &&
					runs.Items[i].Labels["ai.aipipelines.io/pipeline"] == pipelineName {
					found = &runs.Items[i]
					break
				}
			}
			Expect(found).NotTo(BeNil())
			Expect(found.Spec.PipelineRef).To(Equal(pipelineName))
			Expect(found.Spec.Description).To(Equal(prompt))
			Expect(found.Spec.IssueKey).To(BeEmpty())
			Expect(found.Spec.IssueNumber).To(BeZero())

			// Verify owner reference
			Expect(found.OwnerReferences).To(HaveLen(1))
			Expect(found.OwnerReferences[0].Name).To(Equal(pipelineName))
		})
	})
})
