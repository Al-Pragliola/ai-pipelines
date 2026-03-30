package trigger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchGitHubPRDiff_Success(t *testing.T) {
	expectedDiff := `diff --git a/main.go b/main.go
index 1234567..abcdefg 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {
+    fmt.Println("hello")
 }
`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request path
		if r.URL.Path != "/repos/test-owner/test-repo/pulls/42" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		// Verify Accept header requests diff format
		accept := r.Header.Get("Accept")
		if accept != "application/vnd.github.diff" {
			t.Errorf("expected Accept header 'application/vnd.github.diff', got %q", accept)
		}
		// Verify Authorization header
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("expected Authorization 'Bearer test-token', got %q", auth)
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(expectedDiff)) //nolint:errcheck
	}))
	defer srv.Close()

	diff, err := FetchGitHubPRDiff(context.Background(), srv.URL, "test-owner", "test-repo", 42, "test-token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != expectedDiff {
		t.Errorf("diff mismatch:\ngot:  %q\nwant: %q", diff, expectedDiff)
	}
}

func TestFetchGitHubPRDiff_Truncation(t *testing.T) {
	// Create a diff larger than 100KB
	largeDiff := strings.Repeat("a", 200*1024) // 200KB

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeDiff)) //nolint:errcheck
	}))
	defer srv.Close()

	diff, err := FetchGitHubPRDiff(context.Background(), srv.URL, "owner", "repo", 1, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	maxSize := 100 * 1024 // 100KB
	if len(diff) > maxSize {
		t.Errorf("diff should be truncated to %d bytes, got %d", maxSize, len(diff))
	}
	if len(diff) == 0 {
		t.Error("diff should not be empty after truncation")
	}
}

func TestFetchGitHubPRDiff_ExactlyAtCap(t *testing.T) {
	// A diff exactly at the cap should not be truncated
	maxSize := 100 * 1024
	exactDiff := strings.Repeat("x", maxSize)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(exactDiff)) //nolint:errcheck
	}))
	defer srv.Close()

	diff, err := FetchGitHubPRDiff(context.Background(), srv.URL, "owner", "repo", 1, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(diff) != maxSize {
		t.Errorf("diff at exactly the cap should not be modified, got length %d", len(diff))
	}
}

func TestFetchGitHubPRDiff_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := FetchGitHubPRDiff(context.Background(), srv.URL, "owner", "repo", 999, "token")
	if err == nil {
		t.Fatal("expected error for non-200 response, got nil")
	}
}

func TestFetchGitHubPRDiff_EmptyDiff(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Empty body
	}))
	defer srv.Close()

	diff, err := FetchGitHubPRDiff(context.Background(), srv.URL, "owner", "repo", 1, "token")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff != "" {
		t.Errorf("expected empty diff, got %q", diff)
	}
}

func TestFetchGitHubPRDiff_ContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("diff content")) //nolint:errcheck
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := FetchGitHubPRDiff(ctx, srv.URL, "owner", "repo", 1, "token")
	if err == nil {
		t.Fatal("expected error for canceled context, got nil")
	}
}
