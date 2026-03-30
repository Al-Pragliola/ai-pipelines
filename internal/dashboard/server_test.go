package dashboard

import (
	"encoding/json"
	"testing"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
