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

func TestFetchGitHubPRs_ExcludesAuthors(t *testing.T) {
	pulls := []map[string]any{
		{
			"number": 1,
			"title":  "Human PR",
			"body":   "Real work",
			"user":   map[string]any{"login": "developer"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "feat/thing"},
		},
		{
			"number": 2,
			"title":  "Bump deps",
			"body":   "Automated",
			"user":   map[string]any{"login": "dependabot[bot]"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "dependabot/npm/foo"},
		},
		{
			"number": 3,
			"title":  "Another human PR",
			"body":   "More work",
			"user":   map[string]any{"login": "contributor"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "fix/bug"},
		},
		{
			"number": 4,
			"title":  "Renovate update",
			"body":   "Also automated",
			"user":   map[string]any{"login": "renovate[bot]"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "renovate/something"},
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

	spec := &aiv1alpha1.GitHubPRTriggerSpec{
		Owner:          "test-owner",
		Repo:           "test-repo",
		ExcludeAuthors: []string{"dependabot[bot]", "renovate[bot]"},
	}

	issues, err := FetchGitHubPRs(context.Background(), spec, "test-token", NewCachedClient())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues, got %d", len(issues))
	}

	if issues[0].Number != 1 {
		t.Errorf("expected Number 1, got %d", issues[0].Number)
	}
	if issues[0].PRAuthor != "developer" {
		t.Errorf("expected PRAuthor 'developer', got %q", issues[0].PRAuthor)
	}
	if issues[0].BaseBranch != "main" {
		t.Errorf("expected BaseBranch 'main', got %q", issues[0].BaseBranch)
	}
	if issues[0].HeadBranch != "feat/thing" {
		t.Errorf("expected HeadBranch 'feat/thing', got %q", issues[0].HeadBranch)
	}

	if issues[1].Number != 3 {
		t.Errorf("expected Number 3, got %d", issues[1].Number)
	}
	if issues[1].PRAuthor != "contributor" {
		t.Errorf("expected PRAuthor 'contributor', got %q", issues[1].PRAuthor)
	}
}

func TestFetchGitHubPRs_NoExcludeAuthors(t *testing.T) {
	pulls := []map[string]any{
		{
			"number": 1,
			"title":  "PR one",
			"body":   "body",
			"user":   map[string]any{"login": "anyone"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "branch-1"},
		},
		{
			"number": 2,
			"title":  "PR two",
			"body":   "body",
			"user":   map[string]any{"login": "bot"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "branch-2"},
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

	spec := &aiv1alpha1.GitHubPRTriggerSpec{
		Owner: "owner",
		Repo:  "repo",
	}

	issues, err := FetchGitHubPRs(context.Background(), spec, "token", NewCachedClient())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 2 {
		t.Fatalf("expected 2 issues (no filtering), got %d", len(issues))
	}
}

func TestFetchGitHubPRs_KeyFormat(t *testing.T) {
	pulls := []map[string]any{
		{
			"number": 42,
			"title":  "Test PR",
			"body":   "body",
			"user":   map[string]any{"login": "dev"},
			"base":   map[string]any{"ref": "main"},
			"head":   map[string]any{"ref": "feat"},
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

	spec := &aiv1alpha1.GitHubPRTriggerSpec{
		Owner: "owner",
		Repo:  "repo",
	}

	issues, err := FetchGitHubPRs(context.Background(), spec, "token", NewCachedClient())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(issues) != 1 {
		t.Fatalf("expected 1 issue, got %d", len(issues))
	}
	if issues[0].Key != "#PR-42" {
		t.Errorf("expected key '#PR-42', got %q", issues[0].Key)
	}
}

func TestFetchGitHubPRs_URLAndHeaders(t *testing.T) {
	var capturedPath string
	var capturedQuery string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]map[string]any{}) //nolint:errcheck
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRTriggerSpec{
		Owner: "myorg",
		Repo:  "myrepo",
	}

	_, err := FetchGitHubPRs(context.Background(), spec, "token", NewCachedClient())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedPath := "/repos/myorg/myrepo/pulls"
	if capturedPath != expectedPath {
		t.Errorf("expected path %q, got %q", expectedPath, capturedPath)
	}

	if !strings.Contains(capturedQuery, "state=open") {
		t.Errorf("expected query to contain state=open, got %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "sort=created") {
		t.Errorf("expected query to contain sort=created, got %q", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "direction=desc") {
		t.Errorf("expected query to contain direction=desc, got %q", capturedQuery)
	}
}

func TestFetchGitHubPRs_APIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	originalBaseURL := gitHubAPIBaseURL
	gitHubAPIBaseURL = server.URL
	defer func() { gitHubAPIBaseURL = originalBaseURL }()

	spec := &aiv1alpha1.GitHubPRTriggerSpec{
		Owner: "owner",
		Repo:  "repo",
	}

	_, err := FetchGitHubPRs(context.Background(), spec, "bad-token", NewCachedClient())
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}
