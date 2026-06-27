package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// A5: getRaw (used by FileContent) must return *RateLimitError on a 403 with
// X-RateLimit-Remaining: 0, matching the behaviour of getWithHeader. Before
// this fix it fell through to a generic error, misreporting rate-limit hits
// during workflow-file fetches as partial failures.
func TestGetRaw_403RateLimited_ReturnsRateLimitError(t *testing.T) {
	reset := time.Now().Add(30 * time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Limit", "60")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.Header().Set("X-RateLimit-Resource", "core")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"rate limited"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FileContent(context.Background(), "acme", "widget", ".github/workflows/ci.yml")
	if err == nil {
		t.Fatal("expected rate-limit error for getRaw on 403; got nil")
	}
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Errorf("want *RateLimitError for getRaw 403 with remaining=0; got %T: %v", err, err)
	}
	if rle.Limit != 60 {
		t.Errorf("RateLimitError.Limit = %d; want 60", rle.Limit)
	}
}

// A5: a 429 (TooManyRequests) with remaining=0 on getRaw must also yield a
// *RateLimitError, not a generic error.
func TestGetRaw_429RateLimited_ReturnsRateLimitError(t *testing.T) {
	reset := time.Now().Add(time.Minute).Unix()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Limit", "5000")
		w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(reset, 10))
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FileContent(context.Background(), "acme", "widget", ".github/workflows/ci.yml")
	var rle *RateLimitError
	if !errors.As(err, &rle) {
		t.Errorf("want *RateLimitError for getRaw 429; got %T: %v", err, err)
	}
}

// A5: a 403 WITHOUT remaining=0 must NOT be a *RateLimitError (generic error only).
func TestGetRaw_403WithoutRemainingZeroIsGenericError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "50")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Must have push access"}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	_, err := c.FileContent(context.Background(), "acme", "widget", ".github/workflows/ci.yml")
	if err == nil {
		t.Fatal("expected error for 403")
	}
	var rle *RateLimitError
	if errors.As(err, &rle) {
		t.Error("403 without remaining=0 must NOT be a *RateLimitError")
	}
}
