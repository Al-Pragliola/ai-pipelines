package trigger

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// CachedClient wraps HTTP GET requests with ETag-based conditional requests
// and retry with exponential backoff on 503/429. GitHub does not count 304
// responses against the rate limit, so polling the same endpoint repeatedly
// becomes nearly free when the data hasn't changed.
type CachedClient struct {
	mu    sync.Mutex
	cache map[string]*cacheEntry

	// MaxRetries is the number of additional attempts after the first request
	// fails with 503 or 429. Defaults to 2 (3 total attempts).
	MaxRetries int
}

type cacheEntry struct {
	etag string
	body []byte
}

// NewCachedClient creates a CachedClient with default settings.
func NewCachedClient() *CachedClient {
	return &CachedClient{
		cache:      make(map[string]*cacheEntry),
		MaxRetries: 2,
	}
}

// Get performs an HTTP GET with ETag caching and retry on transient errors.
// It returns the response body and any error. On 304 Not Modified, the cached
// body from the previous successful response is returned. Non-2xx responses
// (after retries) are returned as errors.
func (c *CachedClient) Get(ctx context.Context, url, token, accept string) ([]byte, error) {
	c.mu.Lock()
	entry := c.cache[url]
	c.mu.Unlock()

	maxRetries := c.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	var lastStatusCode int
	for attempt := range maxRetries + 1 {
		if attempt > 0 {
			backoff := time.Duration(attempt) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Accept", accept)
		if entry != nil {
			req.Header.Set("If-None-Match", entry.etag)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close() //nolint:errcheck

		lastStatusCode = resp.StatusCode

		if resp.StatusCode == http.StatusNotModified && entry != nil {
			return entry.body, nil
		}

		if resp.StatusCode == http.StatusServiceUnavailable || resp.StatusCode == http.StatusTooManyRequests {
			continue // retry
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("github api returned %d", resp.StatusCode)
		}

		if err != nil {
			return nil, err
		}

		// Cache the response if an ETag was provided.
		if etag := resp.Header.Get("ETag"); etag != "" {
			c.mu.Lock()
			c.cache[url] = &cacheEntry{etag: etag, body: body}
			c.mu.Unlock()
		}

		return body, nil
	}

	return nil, fmt.Errorf("github api returned %d after %d retries", lastStatusCode, maxRetries)
}
