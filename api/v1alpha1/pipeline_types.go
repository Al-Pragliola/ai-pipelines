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

package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineSpec defines the desired state of Pipeline.
type PipelineSpec struct {
	// repo configures the git repository for checkout/push steps.
	// Required unless the pipeline uses a triage step to select the repo.
	// +optional
	Repo *RepoSpec `json:"repo,omitempty"`

	// repos is a list of candidate repositories for the triage step.
	// The triage step selects which repo to use based on the ticket content.
	// +optional
	Repos []RepoCandidate `json:"repos,omitempty"`

	// trigger configures how new pipeline runs are created.
	// +required
	Trigger TriggerSpec `json:"trigger"`

	// ai configures the AI runtime (container image, env, secrets).
	// +required
	AI AISpec `json:"ai"`

	// steps defines the ordered list of pipeline steps.
	// +required
	// +kubebuilder:validation:MinItems=1
	Steps []StepSpec `json:"steps"`
}

// RepoSpec configures the git repository for checkout and push steps.
type RepoSpec struct {
	// owner is the GitHub repository owner.
	// +required
	Owner string `json:"owner"`

	// name is the GitHub repository name.
	// +required
	Name string `json:"name"`

	// forkOwner is the owner of the fork to push to. Defaults to owner.
	// +optional
	ForkOwner string `json:"forkOwner,omitempty"`

	// secretRef references a K8s Secret containing the git token.
	// The secret must have a key named "token".
	// +required
	SecretRef SecretKeyRef `json:"secretRef"`
}

// RepoCandidate describes a candidate repository for triage.
type RepoCandidate struct {
	// owner is the GitHub repository owner.
	// +required
	Owner string `json:"owner"`

	// name is the GitHub repository name.
	// +required
	Name string `json:"name"`

	// description helps the AI understand what this repo is for.
	// +optional
	Description string `json:"description,omitempty"`

	// forkOwner is the owner of the fork to push to. Defaults to owner.
	// +optional
	ForkOwner string `json:"forkOwner,omitempty"`

	// secretRef references a K8s Secret containing the git token.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`
}

// TriggerSpec configures the pipeline trigger.
type TriggerSpec struct {
	// github configures GitHub Issues polling.
	// +optional
	GitHub *GitHubTriggerSpec `json:"github,omitempty"`

	// jira configures Jira Issues polling.
	// +optional
	Jira *JiraTriggerSpec `json:"jira,omitempty"`
}

// GitHubTriggerSpec configures GitHub Issues trigger.
type GitHubTriggerSpec struct {
	// owner is the GitHub repository owner (for issue fetching).
	// +required
	Owner string `json:"owner"`

	// repo is the GitHub repository name (for issue fetching).
	// +required
	Repo string `json:"repo"`

	// assignee filters issues by this assignee.
	// +required
	Assignee string `json:"assignee"`

	// pollInterval is how often to check for new issues (e.g. "30s", "5m").
	// +optional
	// +kubebuilder:default="30s"
	PollInterval string `json:"pollInterval,omitempty"`

	// labels filters issues by these labels. Empty means no label filter.
	// +optional
	Labels []string `json:"labels,omitempty"`

	// secretRef references a K8s Secret containing the GitHub token.
	// The secret must have a key named "token".
	// +required
	SecretRef SecretKeyRef `json:"secretRef"`
}

// JiraTriggerSpec configures Jira Issues trigger.
type JiraTriggerSpec struct {
	// url is the Jira instance URL (e.g. "https://mycompany.atlassian.net").
	// +required
	URL string `json:"url"`

	// jql is the JQL query to find issues (e.g. 'project = PROJ AND assignee = currentUser()').
	// +required
	JQL string `json:"jql"`

	// pollInterval is how often to check for new issues (e.g. "30s", "5m").
	// +optional
	// +kubebuilder:default="60s"
	PollInterval string `json:"pollInterval,omitempty"`

	// secretRef references a K8s Secret containing Jira credentials.
	// For Jira Cloud: key "email" + key "token" (API token, Basic auth).
	// For Jira Server/DC: key "token" (PAT, Bearer auth).
	// +required
	SecretRef SecretKeyRef `json:"secretRef"`
}

// SecretKeyRef references a key in a K8s Secret.
type SecretKeyRef struct {
	// name is the name of the Secret.
	// +required
	Name string `json:"name"`

	// key is the key within the Secret. Defaults to "token".
	// +optional
	// +kubebuilder:default="token"
	Key string `json:"key,omitempty"`
}

// AISpec configures the AI container runtime.
type AISpec struct {
	// image is the container image for AI steps (e.g. "ai-pipelines-claude:latest").
	// +required
	Image string `json:"image"`

	// env is additional environment variables for the AI container.
	// +optional
	Env map[string]string `json:"env,omitempty"`

	// secretRef references a K8s Secret to mount as credentials into AI pods.
	// +optional
	SecretRef *SecretKeyRef `json:"secretRef,omitempty"`

	// credentialsMountPath is where to mount the credentials file inside the AI container.
	// +optional
	// +kubebuilder:default="/tmp/gcp-creds.json"
	CredentialsMountPath string `json:"credentialsMountPath,omitempty"`

	// imagePullPolicy for the AI container. Set to "Never" for pre-loaded images (e.g. kind).
	// +optional
	// +kubebuilder:default="IfNotPresent"
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

// StepSpec defines a single pipeline step.
type StepSpec struct {
	// name is the step identifier.
	// +required
	Name string `json:"name"`

	// type is the step type.
	// +required
	// +kubebuilder:validation:Enum=git-checkout;ai;shell;git-push;triage
	Type string `json:"type"`

	// branchTemplate is a Go template for the branch name (git-checkout only).
	// +optional
	BranchTemplate string `json:"branchTemplate,omitempty"`

	// promptTemplate is a Go template for the AI prompt (ai and triage steps).
	// +optional
	PromptTemplate string `json:"promptTemplate,omitempty"`

	// commands is a list of shell commands to run (shell only).
	// +optional
	Commands []string `json:"commands,omitempty"`

	// image overrides the default image for this step (shell only).
	// +optional
	Image string `json:"image,omitempty"`

	// dind enables a Docker-in-Docker sidecar for this step.
	// Allows running docker, kind, and container-based tests.
	// Requires a privileged sidecar — use only when needed.
	// +optional
	DinD bool `json:"dind,omitempty"`

	// failureFile is a path to a file that, if it exists after the AI step completes,
	// causes the step to be marked as failed. Used to detect test failures — the AI
	// writes a report to this file, and the container exits non-zero.
	// +optional
	FailureFile string `json:"failureFile,omitempty"`

	// requireApproval pauses the pipeline before running this step and waits
	// for explicit user approval via the dashboard. Useful as a gate before
	// pushing AI-generated code.
	// +optional
	RequireApproval bool `json:"requireApproval,omitempty"`

	// onFailure is the step name to jump back to on failure.
	// +optional
	OnFailure string `json:"onFailure,omitempty"`

	// maxRetries is the maximum number of retry attempts.
	// +optional
	// +kubebuilder:default=0
	MaxRetries int `json:"maxRetries,omitempty"`

	// confidenceThreshold is the minimum confidence for the triage step
	// to auto-select a repo (0.0-1.0). Below this, pipeline waits for user input.
	// +optional
	// +kubebuilder:default="0.7"
	ConfidenceThreshold string `json:"confidenceThreshold,omitempty"`
}

// PipelineStatus defines the observed state of Pipeline.
type PipelineStatus struct {
	// pollerActive indicates whether the trigger poller is running.
	// +optional
	PollerActive bool `json:"pollerActive,omitempty"`

	// lastPollTime is the last time the trigger polled for issues.
	// +optional
	LastPollTime *metav1.Time `json:"lastPollTime,omitempty"`

	// conditions represent the current state of the Pipeline.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Trigger",type=string,JSONPath=`.status.triggerType`,priority=0
// +kubebuilder:printcolumn:name="Poller",type=boolean,JSONPath=`.status.pollerActive`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// Pipeline is the Schema for the pipelines API.
type Pipeline struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PipelineSpec `json:"spec"`

	// +optional
	Status PipelineStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PipelineList contains a list of Pipeline.
type PipelineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []Pipeline `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Pipeline{}, &PipelineList{})
}
