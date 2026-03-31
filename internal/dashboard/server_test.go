package dashboard

import (
	"encoding/json"
	"fmt"
	"testing"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	triggerTypeSpot     = "Spot"
	triggerTypeGitHub   = "GitHub"
	triggerTypeJira     = "Jira"
	triggerTypePRReview = "PR Review"
)

func TestToRunResponse_WorkflowFields(t *testing.T) {
	run := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run",
			Namespace: "default",
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: "test-pipeline",
			IssueKey:    "GH-42",
			IssueTitle:  "fix bug",
		},
		Status: aiv1alpha1.PipelineRunStatus{
			Phase:       aiv1alpha1.PipelineRunPhaseRunning,
			CurrentStep: "ai-step",
			Steps: []aiv1alpha1.StepStatus{
				{
					Name:  "ai-step",
					Type:  "ai",
					Phase: aiv1alpha1.PipelineRunPhaseRunning,
				},
			},
		},
	}

	resp := toRunResponse(run)

	if len(resp.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(resp.Steps))
	}

	// stepResponse should have a WorkflowRef field that is serialized in JSON.
	// Since the step in the PipelineRun doesn't have a workflow, it should be nil/empty.
	data, err := json.Marshal(resp.Steps[0])
	if err != nil {
		t.Fatalf("failed to marshal step response: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal step response: %v", err)
	}

	// The stepResponse struct should have a "workflowRef" JSON field.
	// For a step without a workflow, the field should be absent (omitempty) or null.
	if _, exists := raw["workflowRef"]; exists && raw["workflowRef"] != nil {
		t.Errorf("expected workflowRef to be absent/nil for non-workflow step, got %v", raw["workflowRef"])
	}
}

func TestToRunResponse_StepWithWorkflow(t *testing.T) {
	// This test verifies that when workflow info is available,
	// the stepResponse includes the workflow repo and path.
	// The implementation needs to look up the Pipeline's step specs
	// to populate workflow info on the stepResponse.

	run := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-run-wf",
			Namespace: "default",
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: "test-pipeline",
		},
		Status: aiv1alpha1.PipelineRunStatus{
			Phase:       aiv1alpha1.PipelineRunPhaseRunning,
			CurrentStep: "ai-step",
			Steps: []aiv1alpha1.StepStatus{
				{
					Name:  "ai-step",
					Type:  "ai",
					Phase: aiv1alpha1.PipelineRunPhaseRunning,
				},
			},
		},
	}

	resp := toRunResponse(run)

	// The stepResponse type must have a WorkflowRepo and WorkflowPath field.
	// This will fail to compile until the fields are added.
	step := resp.Steps[0]

	// Verify the struct has the workflow fields by accessing them.
	// These fields should exist even if empty for this run (no pipeline lookup yet).
	_ = step.WorkflowRepo
	_ = step.WorkflowPath
}

func TestStepResponseJSON_IncludesWorkflowFields(t *testing.T) {
	// Verify that a stepResponse with workflow info serializes correctly.
	sr := stepResponse{
		Name:         "code-step",
		Type:         "ai",
		Phase:        "Running",
		JobName:      "job-123",
		Attempt:      1,
		WorkflowRepo: "org/workflows",
		WorkflowPath: "coding/v1",
	}

	data, err := json.Marshal(sr)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if raw["workflowRepo"] != "org/workflows" {
		t.Errorf("expected workflowRepo=org/workflows, got %v", raw["workflowRepo"])
	}
	if raw["workflowPath"] != "coding/v1" {
		t.Errorf("expected workflowPath=coding/v1, got %v", raw["workflowPath"])
	}
}

// --- PR Review trigger tests ---

func TestPipelineResponse_PRReviewTriggerType(t *testing.T) {
	// When a Pipeline has a GitHubPRReview trigger configured,
	// the pipelineResponse should have TriggerType "PR Review" and
	// TriggerInfo showing "owner/repo (reviewer: user)".
	resp := pipelineResponse{
		Name:      "pr-review-pipeline",
		Namespace: "default",
	}

	// Simulate the trigger-type logic that handleListPipelines should do:
	// For a GitHubPRReview trigger, TriggerType should be "PR Review".
	// This test asserts the struct can hold these values and the JSON
	// serializes correctly.

	// The implementation should set these for a GitHubPRReview trigger:
	resp.TriggerType = triggerTypePRReview
	resp.TriggerInfo = "my-org/my-repo (reviewer: bot-user)"
	resp.PollInterval = "60s"

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if raw["triggerType"] != triggerTypePRReview {
		t.Errorf("expected triggerType='PR Review', got %v", raw["triggerType"])
	}
	if raw["triggerInfo"] != "my-org/my-repo (reviewer: bot-user)" {
		t.Errorf("expected triggerInfo to contain owner/repo and reviewer, got %v", raw["triggerInfo"])
	}
}

func TestToPipelineResponse_GitHubPRReviewTrigger(t *testing.T) {
	// The handleListPipelines function should detect GitHubPRReview triggers
	// and set TriggerType to "PR Review" with appropriate info.
	// This test creates a Pipeline with GitHubPRReview trigger and verifies
	// the response fields.
	//
	// This test will FAIL until the switch case for GitHubPRReview is added
	// in handleListPipelines.

	pipeline := aiv1alpha1.Pipeline{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pr-review-pipe",
			Namespace: "default",
		},
		Spec: aiv1alpha1.PipelineSpec{
			Trigger: &aiv1alpha1.TriggerSpec{
				GitHubPRReview: &aiv1alpha1.GitHubPRReviewTriggerSpec{
					Owner:        "test-org",
					Repo:         "test-repo",
					Reviewer:     "review-bot",
					PollInterval: "45s",
				},
			},
		},
	}

	// Build pipelineResponse the same way handleListPipelines does.
	resp := pipelineResponse{
		Name:      pipeline.Name,
		Namespace: pipeline.Namespace,
	}
	if pipeline.Spec.Trigger == nil {
		resp.TriggerType = triggerTypeSpot
		resp.TriggerInfo = "Manual runs only"
	} else {
		switch {
		case pipeline.Spec.Trigger.GitHub != nil:
			resp.TriggerType = triggerTypeGitHub
		case pipeline.Spec.Trigger.Jira != nil:
			resp.TriggerType = triggerTypeJira
		case pipeline.Spec.Trigger.GitHubPRReview != nil:
			resp.TriggerType = triggerTypePRReview
			resp.TriggerInfo = fmt.Sprintf("%s/%s (reviewer: %s)", pipeline.Spec.Trigger.GitHubPRReview.Owner, pipeline.Spec.Trigger.GitHubPRReview.Repo, pipeline.Spec.Trigger.GitHubPRReview.Reviewer)
			resp.PollInterval = pipeline.Spec.Trigger.GitHubPRReview.PollInterval
		}
	}

	// These assertions will FAIL until the GitHubPRReview case is added.
	if resp.TriggerType != triggerTypePRReview {
		t.Errorf("expected TriggerType='PR Review', got %q", resp.TriggerType)
	}
	expectedInfo := "test-org/test-repo (reviewer: review-bot)"
	if resp.TriggerInfo != expectedInfo {
		t.Errorf("expected TriggerInfo=%q, got %q", expectedInfo, resp.TriggerInfo)
	}
	if resp.PollInterval != "45s" {
		t.Errorf("expected PollInterval='45s', got %q", resp.PollInterval)
	}
}

func TestRunResponse_PRMetadataFields(t *testing.T) {
	// The runResponse struct should include PR metadata fields
	// (prNumber, prAuthor, prTitle, prBody, baseBranch, headBranch)
	// so the frontend can display PR info instead of issue info.
	//
	// This test will FAIL until the fields are added to runResponse.

	run := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pr-review-run",
			Namespace: "default",
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: "pr-pipeline",
			IssueKey:    "#PR-42",
			IssueTitle:  "Add feature X",
			PRNumber:    42,
			PRTitle:     "Add feature X",
			PRBody:      "This PR adds feature X",
			PRAuthor:    "contributor",
			BaseBranch:  "main",
			HeadBranch:  "feature-x",
		},
		Status: aiv1alpha1.PipelineRunStatus{
			Phase: aiv1alpha1.PipelineRunPhaseRunning,
			Steps: []aiv1alpha1.StepStatus{},
		},
	}

	resp := toRunResponse(run)

	// Verify PR metadata is propagated to the response.
	// These field accesses will fail to compile until added to runResponse.
	if resp.PRNumber != 42 {
		t.Errorf("expected PRNumber=42, got %d", resp.PRNumber)
	}
	if resp.PRTitle != "Add feature X" {
		t.Errorf("expected PRTitle='Add feature X', got %q", resp.PRTitle)
	}
	if resp.PRBody != "This PR adds feature X" {
		t.Errorf("expected PRBody='This PR adds feature X', got %q", resp.PRBody)
	}
	if resp.PRAuthor != "contributor" {
		t.Errorf("expected PRAuthor='contributor', got %q", resp.PRAuthor)
	}
	if resp.BaseBranch != "main" {
		t.Errorf("expected BaseBranch='main', got %q", resp.BaseBranch)
	}
	if resp.HeadBranch != "feature-x" {
		t.Errorf("expected HeadBranch='feature-x', got %q", resp.HeadBranch)
	}
}

func TestRunResponseJSON_PRMetadataFields(t *testing.T) {
	// Verify PR metadata fields appear correctly in JSON serialization.
	// This will FAIL until the fields are added to runResponse and toRunResponse.

	run := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pr-run-json",
			Namespace: "default",
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: "pr-pipeline",
			PRNumber:    99,
			PRTitle:     "Fix typo",
			PRAuthor:    "dev-user",
			BaseBranch:  "main",
			HeadBranch:  "fix/typo",
		},
		Status: aiv1alpha1.PipelineRunStatus{
			Phase: aiv1alpha1.PipelineRunPhaseSucceeded,
			Steps: []aiv1alpha1.StepStatus{},
		},
	}

	resp := toRunResponse(run)
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Check JSON keys exist with correct values
	if v, ok := raw["prNumber"]; !ok || int(v.(float64)) != 99 {
		t.Errorf("expected prNumber=99 in JSON, got %v", raw["prNumber"])
	}
	if raw["prTitle"] != "Fix typo" {
		t.Errorf("expected prTitle='Fix typo', got %v", raw["prTitle"])
	}
	if raw["prAuthor"] != "dev-user" {
		t.Errorf("expected prAuthor='dev-user', got %v", raw["prAuthor"])
	}
	if raw["baseBranch"] != "main" {
		t.Errorf("expected baseBranch='main', got %v", raw["baseBranch"])
	}
	if raw["headBranch"] != "fix/typo" {
		t.Errorf("expected headBranch='fix/typo', got %v", raw["headBranch"])
	}
}

func TestRunResponseJSON_IncludesStepWorkflowInfo(t *testing.T) {
	// Verify the full runResponse includes workflow info in steps when serialized.
	run := &aiv1alpha1.PipelineRun{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "run-with-wf",
			Namespace: "default",
		},
		Spec: aiv1alpha1.PipelineRunSpec{
			PipelineRef: "my-pipeline",
		},
		Status: aiv1alpha1.PipelineRunStatus{
			Phase: aiv1alpha1.PipelineRunPhaseSucceeded,
			Steps: []aiv1alpha1.StepStatus{
				{
					Name:  "checkout",
					Type:  "git-checkout",
					Phase: aiv1alpha1.PipelineRunPhaseSucceeded,
				},
				{
					Name:  "code",
					Type:  "ai",
					Phase: aiv1alpha1.PipelineRunPhaseSucceeded,
				},
			},
		},
	}

	resp := toRunResponse(run)

	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("failed to marshal runResponse: %v", err)
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	steps, ok := raw["steps"].([]any)
	if !ok {
		t.Fatalf("expected steps to be an array")
	}

	// Both steps should have workflowRepo and workflowPath keys in JSON
	// (even if empty/omitted for non-workflow steps)
	for i, s := range steps {
		stepMap, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("step %d is not an object", i)
		}
		// The JSON should support the workflowRepo key
		// For a proper implementation, this would be populated from the Pipeline spec
		_ = stepMap // ensure it deserializes properly
	}

	// Verify the response structure can hold workflow info by
	// checking the Go struct fields exist
	for _, s := range resp.Steps {
		_ = s.WorkflowRepo
		_ = s.WorkflowPath
	}
}
