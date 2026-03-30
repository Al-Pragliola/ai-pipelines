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
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("Sample Pipeline with workflow", func() {
	const samplePath = "../../config/samples/ai_v1alpha1_pipeline_workflow.yaml"

	Context("Sample file exists and is valid YAML", func() {
		It("should exist at config/samples/ai_v1alpha1_pipeline_workflow.yaml", func() {
			_, err := os.Stat(samplePath)
			Expect(err).NotTo(HaveOccurred(), "sample file must exist at config/samples/ai_v1alpha1_pipeline_workflow.yaml")
		})

		It("should parse as a valid Pipeline CR", func() {
			data, err := os.ReadFile(samplePath)
			Expect(err).NotTo(HaveOccurred())

			pipeline := &aiv1alpha1.Pipeline{}
			err = yaml.UnmarshalStrict(data, pipeline)
			Expect(err).NotTo(HaveOccurred(), "sample must be valid Pipeline YAML")
			Expect(pipeline.Kind).To(Equal("Pipeline"))
			Expect(pipeline.APIVersion).To(Equal("ai.aipipelines.io/v1alpha1"))
		})
	})

	Context("Pipeline spec uses bugfix workflow", func() {
		var pipeline aiv1alpha1.Pipeline

		BeforeEach(func() {
			data, err := os.ReadFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.UnmarshalStrict(data, &pipeline)).To(Succeed())
		})

		It("should have at least one AI step with a workflowRef", func() {
			var found bool
			for _, step := range pipeline.Spec.Steps {
				if step.Type == "ai" && step.WorkflowRef != nil {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "at least one AI step must have a workflowRef")
		})

		It("should reference the ambient-code/workflows repo", func() {
			for _, step := range pipeline.Spec.Steps {
				if step.Type == "ai" && step.WorkflowRef != nil {
					Expect(step.WorkflowRef.Repo).To(Equal("ambient-code/workflows"))
				}
			}
		})

		It("should reference the bugfix workflow path", func() {
			var found bool
			for _, step := range pipeline.Spec.Steps {
				if step.Type == "ai" && step.WorkflowRef != nil && step.WorkflowRef.Path == "bugfix" {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "at least one step must reference the 'bugfix' workflow path")
		})

		It("should have a short promptTemplate with issue context and /speedrun, not full methodology", func() {
			for _, step := range pipeline.Spec.Steps {
				if step.Type == "ai" && step.WorkflowRef != nil && step.WorkflowRef.Path == "bugfix" {
					// The promptTemplate should be concise issue context, not a full methodology
					Expect(step.PromptTemplate).NotTo(BeEmpty(), "promptTemplate must provide issue context")

					// It should contain /speedrun as the command to run
					Expect(step.PromptTemplate).To(ContainSubstring("/speedrun"),
						"promptTemplate should include 'run /speedrun' since the workflow provides methodology")

					// It should be short — issue context only, not a multi-paragraph methodology
					lines := strings.Split(strings.TrimSpace(step.PromptTemplate), "\n")
					Expect(len(lines)).To(BeNumerically("<", 20),
						"promptTemplate should be concise issue context, not full methodology (found %d lines)", len(lines))
				}
			}
		})
	})

	Context("Pipeline can be applied to the API server", func() {
		const resourceName = "test-workflow-sample-pipeline"

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

		It("should be accepted by the API server when loaded from the sample file", func() {
			data, err := os.ReadFile(samplePath)
			Expect(err).NotTo(HaveOccurred())

			pipeline := &aiv1alpha1.Pipeline{}
			Expect(yaml.UnmarshalStrict(data, pipeline)).To(Succeed())

			// Override name/namespace for test isolation
			pipeline.Name = resourceName
			pipeline.Namespace = "default"

			Expect(k8sClient.Create(ctx, pipeline)).To(Succeed())

			var fetched aiv1alpha1.Pipeline
			Expect(k8sClient.Get(ctx, typeNamespacedName, &fetched)).To(Succeed())

			// Verify the workflowRef survived the round-trip
			var workflowStep *aiv1alpha1.StepSpec
			for i := range fetched.Spec.Steps {
				if fetched.Spec.Steps[i].WorkflowRef != nil && fetched.Spec.Steps[i].WorkflowRef.Path == "bugfix" {
					workflowStep = &fetched.Spec.Steps[i]
					break
				}
			}
			Expect(workflowStep).NotTo(BeNil(), "bugfix workflowRef step must survive API round-trip")
			Expect(workflowStep.WorkflowRef.Repo).To(Equal("ambient-code/workflows"))
			Expect(workflowStep.PromptTemplate).To(ContainSubstring("/speedrun"))
		})
	})
})
