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

var _ = Describe("PipelineRun PR Fields", func() {
	Context("When creating a PipelineRun with PR fields", func() {
		const resourceName = "test-pr-run"

		ctx := context.Background()

		key := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		AfterEach(func() {
			resource := &aiv1alpha1.PipelineRun{}
			err := k8sClient.Get(ctx, key, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("should store PR fields when set", func() {
			resource := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					PRNumber:    42,
					PRAuthor:    "octocat",
					BaseBranch:  "main",
					HeadBranch:  "feature/awesome",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			var fetched aiv1alpha1.PipelineRun
			Expect(k8sClient.Get(ctx, key, &fetched)).To(Succeed())
			Expect(fetched.Spec.PRNumber).To(Equal(42))
			Expect(fetched.Spec.PRAuthor).To(Equal("octocat"))
			Expect(fetched.Spec.BaseBranch).To(Equal("main"))
			Expect(fetched.Spec.HeadBranch).To(Equal("feature/awesome"))
		})

		It("should default PR fields to zero values when not set", func() {
			resource := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			var fetched aiv1alpha1.PipelineRun
			Expect(k8sClient.Get(ctx, key, &fetched)).To(Succeed())
			Expect(fetched.Spec.PRNumber).To(Equal(0))
			Expect(fetched.Spec.PRAuthor).To(BeEmpty())
			Expect(fetched.Spec.BaseBranch).To(BeEmpty())
			Expect(fetched.Spec.HeadBranch).To(BeEmpty())
		})

		It("should allow PR fields alongside issue fields", func() {
			resource := &aiv1alpha1.PipelineRun{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: aiv1alpha1.PipelineRunSpec{
					PipelineRef: "test-pipeline",
					IssueNumber: 10,
					IssueKey:    "#10",
					IssueTitle:  "Some issue",
					PRNumber:    99,
					PRAuthor:    "contributor",
					BaseBranch:  "develop",
					HeadBranch:  "fix/bug",
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())

			var fetched aiv1alpha1.PipelineRun
			Expect(k8sClient.Get(ctx, key, &fetched)).To(Succeed())
			Expect(fetched.Spec.IssueNumber).To(Equal(10))
			Expect(fetched.Spec.PRNumber).To(Equal(99))
			Expect(fetched.Spec.PRAuthor).To(Equal("contributor"))
			Expect(fetched.Spec.BaseBranch).To(Equal("develop"))
			Expect(fetched.Spec.HeadBranch).To(Equal("fix/bug"))
		})
	})
})

// Compile-time assertion that PipelineRunSpec has the expected PR fields.
var _ = func() {
	_ = aiv1alpha1.PipelineRunSpec{
		PipelineRef: "pipeline",
		PRNumber:    1,
		PRAuthor:    "author",
		BaseBranch:  "main",
		HeadBranch:  "feature",
	}
}
