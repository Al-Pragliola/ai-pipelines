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

var _ = Describe("Sample Pipeline for PR review", func() {
	const samplePath = "../../config/samples/ai_v1alpha1_pipeline_pr_review.yaml"

	Context("Sample file exists and is valid YAML", func() {
		It("should exist at config/samples/ai_v1alpha1_pipeline_pr_review.yaml", func() {
			_, err := os.Stat(samplePath)
			Expect(err).NotTo(HaveOccurred(), "sample file must exist at config/samples/ai_v1alpha1_pipeline_pr_review.yaml")
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

	Context("Pipeline uses githubPRReview trigger", func() {
		var pipeline aiv1alpha1.Pipeline

		BeforeEach(func() {
			data, err := os.ReadFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.UnmarshalStrict(data, &pipeline)).To(Succeed())
		})

		It("should have a githubPRReview trigger configured", func() {
			Expect(pipeline.Spec.Trigger).NotTo(BeNil(), "trigger must be set")
			Expect(pipeline.Spec.Trigger.GitHubPRReview).NotTo(BeNil(), "githubPRReview trigger must be configured")
		})

		It("should not have a github issues trigger", func() {
			Expect(pipeline.Spec.Trigger.GitHub).To(BeNil(), "github issues trigger should not be set for a PR review pipeline")
		})

		It("should have owner, repo, and reviewer fields set", func() {
			pr := pipeline.Spec.Trigger.GitHubPRReview
			Expect(pr.Owner).NotTo(BeEmpty(), "owner must be set")
			Expect(pr.Repo).NotTo(BeEmpty(), "repo must be set")
			Expect(pr.Reviewer).NotTo(BeEmpty(), "reviewer must be set")
		})

		It("should reference a secretRef for the GitHub token", func() {
			pr := pipeline.Spec.Trigger.GitHubPRReview
			Expect(pr.SecretRef.Name).NotTo(BeEmpty(), "secretRef.name must be set")
		})
	})

	Context("Pipeline prompt template references PRDiff and writes review.md", func() {
		var pipeline aiv1alpha1.Pipeline

		BeforeEach(func() {
			data, err := os.ReadFile(samplePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(yaml.UnmarshalStrict(data, &pipeline)).To(Succeed())
		})

		It("should have at least one AI step with a promptTemplate containing PRDiff", func() {
			var found bool
			for _, step := range pipeline.Spec.Steps {
				if step.Type == "ai" && step.PromptTemplate != "" {
					if strings.Contains(step.PromptTemplate, "{{.PRDiff}}") {
						found = true
						break
					}
				}
			}
			Expect(found).To(BeTrue(), "at least one AI step must reference {{.PRDiff}} in its promptTemplate")
		})

		It("should instruct writing /workspace/review.md", func() {
			var found bool
			for _, step := range pipeline.Spec.Steps {
				if step.Type == "ai" && step.PromptTemplate != "" {
					if strings.Contains(step.PromptTemplate, "/workspace/review.md") {
						found = true
						break
					}
				}
			}
			Expect(found).To(BeTrue(), "at least one AI step must instruct writing /workspace/review.md")
		})
	})

	Context("Pipeline can be applied to the API server", func() {
		const resourceName = "test-pr-review-sample-pipeline"

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

			// Verify the githubPRReview trigger survived the round-trip
			Expect(fetched.Spec.Trigger).NotTo(BeNil())
			Expect(fetched.Spec.Trigger.GitHubPRReview).NotTo(BeNil(), "githubPRReview trigger must survive API round-trip")
			Expect(fetched.Spec.Trigger.GitHubPRReview.Owner).NotTo(BeEmpty())
			Expect(fetched.Spec.Trigger.GitHubPRReview.Reviewer).NotTo(BeEmpty())
		})
	})
})
