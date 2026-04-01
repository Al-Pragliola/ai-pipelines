package trigger

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestCachedClient_ETagCaching(t *testing.T) {
	var reqCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)

		if inm := r.Header.Get("If-None-Match"); inm == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		// First request: return data with ETag
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Content-Type", "application/json")
		if n > 1 {
			t.Error("expected conditional request on second call, but no If-None-Match was sent")
		}
		w.Write([]byte(`[{"number":1}]`)) //nolint:errcheck
	}))
	defer server.Close()

	client := NewCachedClient()

	// First call: should get the body and cache it
	body, err := client.Get(context.Background(), server.URL+"/repos/o/r/issues", "tok", "application/json")
	if err != nil {
		t.Fatalf("first call failed: %v", err)
	}
	if string(body) != `[{"number":1}]` {
		t.Fatalf("unexpected body: %s", body)
	}

	// Second call: should send If-None-Match and get cached body back
	body, err = client.Get(context.Background(), server.URL+"/repos/o/r/issues", "tok", "application/json")
	if err != nil {
		t.Fatalf("second call failed: %v", err)
	}
	if string(body) != `[{"number":1}]` {
		t.Fatalf("unexpected cached body: %s", body)
	}

	if reqCount.Load() != 2 {
		t.Fatalf("expected 2 requests, got %d", reqCount.Load())
	}
}

func TestCachedClient_RetryOn503(t *testing.T) {
	var reqCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`"ok"`)) //nolint:errcheck
	}))
	defer server.Close()

	client := NewCachedClient()

	body, err := client.Get(context.Background(), server.URL, "tok", "application/json")
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if string(body) != `"ok"` {
		t.Fatalf("unexpected body: %s", body)
	}
	if reqCount.Load() != 3 {
		t.Fatalf("expected 3 requests (1 + 2 retries), got %d", reqCount.Load())
	}
}

func TestCachedClient_RetryOn429(t *testing.T) {
	var reqCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)
		if n == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Write([]byte(`"ok"`)) //nolint:errcheck
	}))
	defer server.Close()

	client := NewCachedClient()

	body, err := client.Get(context.Background(), server.URL, "tok", "application/json")
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if string(body) != `"ok"` {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestCachedClient_ExhaustedRetries(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewCachedClient()

	_, err := client.Get(context.Background(), server.URL, "tok", "application/json")
	if err == nil {
		t.Fatal("expected error after exhausted retries")
	}
}

func TestCachedClient_NonRetryableError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewCachedClient()

	_, err := client.Get(context.Background(), server.URL, "tok", "application/json")
	if err == nil {
		t.Fatal("expected error for 403")
	}
}

func TestCachedClient_SetsAuthAndAcceptHeaders(t *testing.T) {
	var capturedAuth, capturedAccept string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		capturedAccept = r.Header.Get("Accept")
		w.Write([]byte(`{}`)) //nolint:errcheck
	}))
	defer server.Close()

	client := NewCachedClient()
	_, err := client.Get(context.Background(), server.URL, "my-token", "application/vnd.github+json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedAuth != "Bearer my-token" {
		t.Errorf("expected 'Bearer my-token', got %q", capturedAuth)
	}
	if capturedAccept != "application/vnd.github+json" {
		t.Errorf("expected 'application/vnd.github+json', got %q", capturedAccept)
	}
}

func TestCachedClient_CacheUpdatedOnNewETag(t *testing.T) {
	var reqCount atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := reqCount.Add(1)

		switch n {
		case 1:
			w.Header().Set("ETag", `"v1"`)
			w.Write([]byte(`"first"`)) //nolint:errcheck
		case 2:
			// Data changed — return new ETag even though old one was sent
			if r.Header.Get("If-None-Match") != `"v1"` {
				t.Errorf("expected If-None-Match v1, got %q", r.Header.Get("If-None-Match"))
			}
			w.Header().Set("ETag", `"v2"`)
			w.Write([]byte(`"second"`)) //nolint:errcheck
		case 3:
			// Should now send v2
			if r.Header.Get("If-None-Match") != `"v2"` {
				t.Errorf("expected If-None-Match v2, got %q", r.Header.Get("If-None-Match"))
			}
			w.WriteHeader(http.StatusNotModified)
		}
	}))
	defer server.Close()

	client := NewCachedClient()

	body, _ := client.Get(context.Background(), server.URL, "tok", "application/json")
	if string(body) != `"first"` {
		t.Fatalf("expected first, got %s", body)
	}

	body, _ = client.Get(context.Background(), server.URL, "tok", "application/json")
	if string(body) != `"second"` {
		t.Fatalf("expected second, got %s", body)
	}

	body, _ = client.Get(context.Background(), server.URL, "tok", "application/json")
	if string(body) != `"second"` {
		t.Fatalf("expected cached second, got %s", body)
	}
}

func TestCachedClient_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewCachedClient()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := client.Get(ctx, server.URL, "tok", "application/json")
	if err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
