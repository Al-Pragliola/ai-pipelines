package trigger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	aiv1alpha1 "github.com/Al-Pragliola/ai-pipelines/api/v1alpha1"
)

func TestFetchGitHubReviewRequests_FiltersByReviewer(t *testing.T) {
	pulls := []map[string]any{
		{
			"number": 10,
			"title":  "PR with review request",
			"body":   "Please review this",
			"requested_reviewers": []map[string]any{
				{"login": "target-reviewer"},
				{"login": "other-user"},
			},
		},
		{
			"number": 20,
			"title":  "PR without review request",
			"body":   "No review needed",
			"requested_reviewers": []map[string]any{
				{"login": "someone-else"},
			},
		},
		{
			"number": 30,
			"title":  "Another PR for target reviewer",
			"body":   "Also needs review",
			"requested_reviewers": []map[string]any{
				{"login": "target-reviewer"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %s", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pulls) //nolint:errcheck
	}))
	defer server.Close()

	// Override the GitHub API base URL for testing
	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:    "test-owner",
		Repo:     "test-repo",
		Reviewer: "target-reviewer",
	}

	issues, err := FetchGitHubReviewRequests(context.Background(), spec, "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should only return PRs where target-reviewer is in requested_reviewers
	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	// First matching PR
	if issues[0].Number != 10 {
		t.Errorf("expected Number 10, got %d", issues[0].Number)
	}
	if issues[0].Title != "PR with review request" {
		t.Errorf("expected title 'PR with review request', got %q", issues[0].Title)
	}
	if issues[0].Body != "Please review this" {
		t.Errorf("expected body 'Please review this', got %q", issues[0].Body)
	}
	if issues[0].Key != "#PR-10" {
		t.Errorf("expected key '#PR-10', got %q", issues[0].Key)
	}

	// Second matching PR
	if issues[1].Number != 30 {
		t.Errorf("expected Number 30, got %d", issues[1].Number)
	}
	if issues[1].Title != "Another PR for target reviewer" {
		t.Errorf("expected title 'Another PR for target reviewer', got %q", issues[1].Title)
	}
	if issues[1].Body != "Also needs review" {
		t.Errorf("expected body 'Also needs review', got %q", issues[1].Body)
	}
	if issues[1].Key != "#PR-30" {
		t.Errorf("expected key '#PR-30', got %q", issues[1].Key)
	}
}

func TestFetchGitHubReviewRequests_NoMatchingReviewer(t *testing.T) {
	pulls := []map[string]any{
		{
			"number": 1,
			"title":  "Some PR",
			"body":   "body",
			"requested_reviewers": []map[string]any{
				{"login": "other-user"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pulls) //nolint:errcheck
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:    "owner",
		Repo:     "repo",
		Reviewer: "target-reviewer",
	}

	issues, err := FetchGitHubReviewRequests(context.Background(), spec, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 0 {
		t.Fatalf("expected 0 issues, got %d", len(issues))
	}
}

func TestFetchGitHubReviewRequests_EmptyReviewers(t *testing.T) {
	pulls := []map[string]any{
		{
			"number":              5,
			"title":               "PR no reviewers",
			"body":                "body",
			"requested_reviewers": []map[string]any{},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pulls) //nolint:errcheck
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:    "owner",
		Repo:     "repo",
		Reviewer: "reviewer",
	}

	issues, err := FetchGitHubReviewRequests(context.Background(), spec, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 0 {
		t.Fatalf("expected 0 issues, got %d", len(issues))
	}
}

func TestFetchGitHubReviewRequests_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:    "owner",
		Repo:     "repo",
		Reviewer: "reviewer",
	}

	_, err := FetchGitHubReviewRequests(context.Background(), spec, "bad-token")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestFetchGitHubReviewRequests_KeyFormat(t *testing.T) {
	pulls := []map[string]any{
		{
			"number": 42,
			"title":  "Test PR",
			"body":   "body",
			"requested_reviewers": []map[string]any{
				{"login": "myuser"},
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(pulls) //nolint:errcheck
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:    "owner",
		Repo:     "repo",
		Reviewer: "myuser",
	}

	issues, err := FetchGitHubReviewRequests(context.Background(), spec, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}

	// Key must be #PR-{number} to distinguish from issue keys (#N)
	if issues[0].Key != "#PR-42" {
		t.Errorf("expected key '#PR-42', got %q", issues[0].Key)
	}
}

func TestFetchGitHubReviewRequests_URLAndHeaders(t *testing.T) {
	var capturedPath string
	var capturedQuery string
	var capturedAuth string
	var capturedAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRReviewTriggerSpec{
		Owner:    "myorg",
		Repo:     "myrepo",
		Reviewer: "reviewer",
	}

	_, err := FetchGitHubReviewRequests(context.Background(), spec, "my-secret-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the correct endpoint path: /repos/{owner}/{repo}/pulls
	expectedPath := "/repos/myorg/myrepo/pulls"
	if capturedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, capturedPath)
	}

	// Verify state=open query parameter
	if !strings.Contains(capturedQuery, "state=open") {
		t.Errorf("expected query to contain state=open, got %q", capturedQuery)
	}

	// Verify Authorization header
	if capturedAuth != "Bearer my-secret-token" {
		t.Errorf("expected 'Bearer my-secret-token', got %q", capturedAuth)
	}

	// Verify Accept header
	if capturedAccept != "application/vnd.github+json" {
		t.Errorf("expected 'application/vnd.github+json', got %q", capturedAccept)
	}
}
