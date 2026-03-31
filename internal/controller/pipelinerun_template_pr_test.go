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
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

var _ = Describe("templateData PR fields", func() {
	Context("newTemplateData populates PR fields from PipelineRun spec", func() {
		It("should populate PRNumber, PRAuthor, BaseBranch, HeadBranch from the run spec", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-template-test",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					PRNumber:    42,
					PRAuthor:    "octocat",
					BaseBranch:  "main",
					HeadBranch:  "feature/login",
				},
			}

			data := newTemplateData(run, nil)

			Expect(data.PRNumber).To(Equal(42))
			Expect(data.PRAuthor).To(Equal("octocat"))
			Expect(data.BaseBranch).To(Equal("main"))
			Expect(data.HeadBranch).To(Equal("feature/login"))
		})

		It("should populate PRTitle and PRBody from the run spec", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-template-title-body",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					PRNumber:    7,
					PRTitle:     "Add dark mode support",
					PRBody:      "This PR adds dark mode toggle to the settings page.",
					PRAuthor:    "contributor",
					BaseBranch:  "develop",
					HeadBranch:  "feature/dark-mode",
				},
			}

			data := newTemplateData(run, nil)

			Expect(data.PRTitle).To(Equal("Add dark mode support"))
			Expect(data.PRBody).To(Equal("This PR adds dark mode toggle to the settings page."))
		})

		It("should default PR fields to zero values when not set", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-template-defaults",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					IssueNumber: 10,
					IssueTitle:  "Some issue",
				},
			}

			data := newTemplateData(run, nil)

			Expect(data.PRNumber).To(Equal(0))
			Expect(data.PRTitle).To(BeEmpty())
			Expect(data.PRBody).To(BeEmpty())
			Expect(data.PRDiff).To(BeEmpty())
			Expect(data.PRAuthor).To(BeEmpty())
			Expect(data.BaseBranch).To(BeEmpty())
			Expect(data.HeadBranch).To(BeEmpty())
		})

		It("should allow PR fields alongside issue fields", func() {
			run := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pr-template-mixed",
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					IssueNumber: 10,
					IssueTitle:  "Fix login bug",
					IssueBody:   "Login is broken on Safari",
					PRNumber:    99,
					PRTitle:     "Fix: login on Safari",
					PRBody:      "Fixes the Safari login issue",
					PRAuthor:    "dev123",
					BaseBranch:  "main",
					HeadBranch:  "fix/safari-login",
				},
			}

			data := newTemplateData(run, nil)

			// Issue fields still work
			Expect(data.IssueNumber).To(Equal(10))
			Expect(data.IssueTitle).To(Equal("Fix login bug"))
			Expect(data.IssueBody).To(Equal("Login is broken on Safari"))

			// PR fields are populated
			Expect(data.PRNumber).To(Equal(99))
			Expect(data.PRTitle).To(Equal("Fix: login on Safari"))
			Expect(data.PRBody).To(Equal("Fixes the Safari login issue"))
			Expect(data.PRAuthor).To(Equal("dev123"))
			Expect(data.BaseBranch).To(Equal("main"))
			Expect(data.HeadBranch).To(Equal("fix/safari-login"))
		})
	})

	Context("renderString with PR template variables", func() {
		It("should render PR fields in templates", func() {
			data := templateData{
				PRNumber:   42,
				PRTitle:    "Add feature X",
				PRBody:     "This adds feature X",
				PRAuthor:   "octocat",
				BaseBranch: "main",
				HeadBranch: "feature/x",
			}

			result, err := renderString(
				"Review PR #{{.PRNumber}} by {{.PRAuthor}}: {{.PRTitle}} ({{.HeadBranch}} -> {{.BaseBranch}})",
				data,
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal("Review PR #42 by octocat: Add feature X (feature/x -> main)"))
		})

		It("should render PRBody in templates", func() {
			data := templateData{
				PRNumber: 5,
				PRBody:   "Detailed description of changes",
			}

			result, err := renderString("PR #{{.PRNumber}}\n\n{{.PRBody}}", data)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("PR #5"))
			Expect(result).To(ContainSubstring("Detailed description of changes"))
		})

		It("should render PRDiff in templates", func() {
			data := templateData{
				PRNumber: 3,
				PRDiff:   "diff --git a/main.go b/main.go\n+added line",
			}

			result, err := renderString("Diff for PR #{{.PRNumber}}:\n{{.PRDiff}}", data)
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(ContainSubstring("Diff for PR #3"))
			Expect(result).To(ContainSubstring("diff --git"))
		})
	})
})

// Compile-time assertion that templateData has the expected PR fields.
var _ = func() {
	_ = templateData{
		PRNumber:   1,
		PRTitle:    "title",
		PRBody:     "body",
		PRDiff:     "diff",
		PRAuthor:   "author",
		BaseBranch: "main",
		HeadBranch: "feature",
	}
}
