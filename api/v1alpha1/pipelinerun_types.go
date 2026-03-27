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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PipelineRunSpec defines the desired state of PipelineRun.
type PipelineRunSpec struct {
	// pipelineRef is the name of the Pipeline CR this run belongs to.
	// +required
	PipelineRef string `json:"pipelineRef"`

	// issueNumber is the issue number that triggered this run.
	// +required
	IssueNumber int `json:"issueNumber"`

	// issueKey is the issue identifier (e.g. "PROJ-123" for Jira, "#1" for GitHub).
	// +optional
	IssueKey string `json:"issueKey,omitempty"`

	// issueTitle is the title/summary of the triggering issue.
	// +required
	IssueTitle string `json:"issueTitle"`

	// issueBody is the body/description of the triggering issue.
	// +optional
	IssueBody string `json:"issueBody,omitempty"`

	// selectedRepo is the repo chosen by user when triage is not confident.
	// Set by the user or dashboard to resume a WaitingForInput pipeline.
	// +optional
	SelectedRepo *SelectedRepo `json:"selectedRepo,omitempty"`

	// approvedStep is set by the dashboard to approve a step that has requireApproval.
	// The controller checks this against the current waiting step to resume.
	// +optional
	ApprovedStep string `json:"approvedStep,omitempty"`
}

// SelectedRepo identifies the repo selected for this run (by triage or user).
type SelectedRepo struct {
	// owner is the GitHub repository owner.
	// +required
	Owner string `json:"owner"`

	// name is the GitHub repository name.
	// +required
	Name string `json:"name"`

	// forkOwner is the GitHub owner of the fork where changes are pushed.
	// +optional
	ForkOwner string `json:"forkOwner,omitempty"`
}

// PipelineRunPhase represents the current phase of a PipelineRun.
// +kubebuilder:validation:Enum=Pending;Initializing;Running;WaitingForInput;Succeeded;Failed;Stopped;Deleting
type PipelineRunPhase string

const (
	PipelineRunPhasePending         PipelineRunPhase = "Pending"
	PipelineRunPhaseInitializing    PipelineRunPhase = "Initializing"
	PipelineRunPhaseRunning         PipelineRunPhase = "Running"
	PipelineRunPhaseWaitingForInput PipelineRunPhase = "WaitingForInput"
	PipelineRunPhaseSucceeded       PipelineRunPhase = "Succeeded"
	PipelineRunPhaseFailed          PipelineRunPhase = "Failed"
	PipelineRunPhaseStopped         PipelineRunPhase = "Stopped"
	PipelineRunPhaseDeleting        PipelineRunPhase = "Deleting"
)

// TriageResult holds the AI triage decision.
type TriageResult struct {
	// repo is the repository the AI selected (e.g. "owner/name").
	// +optional
	Repo string `json:"repo,omitempty"`

	// confidence is the AI's confidence score (0.0-1.0) as a string.
	// +optional
	Confidence string `json:"confidence,omitempty"`

	// reasoning explains why the AI chose this repo.
	// +optional
	Reasoning string `json:"reasoning,omitempty"`
}

// StepStatus tracks the execution state of a single step.
type StepStatus struct {
	// name is the step name.
	Name string `json:"name"`

	// type is the step type: git-checkout, ai, shell, git-push, triage.
	// +optional
	Type string `json:"type,omitempty"`

	// phase is the current phase: Pending, Running, Succeeded, Failed.
	Phase PipelineRunPhase `json:"phase"`

	// attempt is the current attempt number.
	// +optional
	Attempt int `json:"attempt,omitempty"`

	// jobName is the name of the K8s Job running this step (for AI/shell steps).
	// +optional
	JobName string `json:"jobName,omitempty"`

	// startedAt is when this step started.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// finishedAt is when this step finished.
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// message contains error details on failure.
	// +optional
	Message string `json:"message,omitempty"`
}

// PipelineRunStatus defines the observed state of PipelineRun.
type PipelineRunStatus struct {
	// phase is the overall phase of the pipeline run.
	// +optional
	Phase PipelineRunPhase `json:"phase,omitempty"`

	// currentStep is the name of the currently executing step.
	// +optional
	CurrentStep string `json:"currentStep,omitempty"`

	// branch is the git branch created for this run.
	// +optional
	Branch string `json:"branch,omitempty"`

	// orchestratorJob is the name of the orchestrator Job.
	// +optional
	OrchestratorJob string `json:"orchestratorJob,omitempty"`

	// pvcName is the name of the workspace PVC.
	// +optional
	PVCName string `json:"pvcName,omitempty"`

	// resolvedRepo is the repo being used for this run (from spec.repo or triage).
	// +optional
	ResolvedRepo *SelectedRepo `json:"resolvedRepo,omitempty"`

	// triageResult holds the AI triage decision, if applicable.
	// +optional
	TriageResult *TriageResult `json:"triageResult,omitempty"`

	// waitingFor describes what the pipeline is waiting for (e.g. "repo-selection").
	// +optional
	WaitingFor string `json:"waitingFor,omitempty"`

	// diffJobName is the name of the diff preview Job created when waiting for step approval.
	// +optional
	DiffJobName string `json:"diffJobName,omitempty"`

	// chatPodName is the name of the chat Pod created during the approval phase.
	// +optional
	ChatPodName string `json:"chatPodName,omitempty"`

	// steps tracks status of each step.
	// +optional
	Steps []StepStatus `json:"steps,omitempty"`

	// startedAt is when the run started.
	// +optional
	StartedAt *metav1.Time `json:"startedAt,omitempty"`

	// finishedAt is when the run finished.
	// +optional
	FinishedAt *metav1.Time `json:"finishedAt,omitempty"`

	// conditions represent the current state of the PipelineRun.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Pipeline",type=string,JSONPath=`.spec.pipelineRef`
// +kubebuilder:printcolumn:name="Issue",type=string,JSONPath=`.spec.issueKey`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Step",type=string,JSONPath=`.status.currentStep`
// +kubebuilder:printcolumn:name="Branch",type=string,JSONPath=`.status.branch`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// PipelineRun is the Schema for the pipelineruns API.
type PipelineRun struct {
	metav1.TypeMeta `json:",inline"`

	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// +required
	Spec PipelineRunSpec `json:"spec"`

	// +optional
	Status PipelineRunStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// PipelineRunList contains a list of PipelineRun.
type PipelineRunList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []PipelineRun `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PipelineRun{}, &PipelineRunList{})
}
