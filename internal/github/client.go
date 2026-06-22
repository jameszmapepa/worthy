package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	apiVersion     = "2022-11-28"
	userAgent      = "worthy (https://github.com/jameszmapepa/worthy)"

	// The default Transport caps idle conns per host at 2, serialising concurrent
	// calls and defeating the bounded worker pool.
	// ceiling: must track metrics.maxConcurrency (8); duplicated to avoid an import
	// cycle — keep in sync if concurrency changes.
	maxIdleConnsPerHost = 8
)

// RateLimitError is returned when the GitHub API rate limit is exhausted.
type RateLimitError struct {
	Reset    time.Time
	Limit    int
	Resource string
	Endpoint string
}

func (e *RateLimitError) Error() string {
	wait := time.Until(e.Reset).Round(time.Second)
	return fmt.Sprintf("github rate limit exhausted (limit %d) on %s; resets in %s (at %s)",
		e.Limit, e.Endpoint, wait, e.Reset.Format(time.Kitchen))
}

// NotFoundError is returned for a 404 response.
type NotFoundError struct{ Endpoint string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("not found: %s", e.Endpoint) }

// Client is a minimal GitHub REST client; call NewClient to construct one. Safe for concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
	maxRetries int
	retryWait  time.Duration
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the underlying *http.Client (used by tests).
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.httpClient = h } }

// WithBaseURL overrides the API base URL (used by tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithToken sets an explicit token, overriding the GITHUB_TOKEN env lookup.
func WithToken(t string) Option { return func(c *Client) { c.token = t } }

// WithRetry tunes the 202-recompute retry policy.
func WithRetry(max int, wait time.Duration) Option {
	return func(c *Client) { c.maxRetries = max; c.retryWait = wait }
}

// NewClient builds a Client, falling back to GITHUB_TOKEN if no token is supplied.
func NewClient(opts ...Option) *Client {
	transport := &http.Transport{}
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		transport = dt.Clone()
	}
	transport.MaxIdleConnsPerHost = maxIdleConnsPerHost
	transport.MaxConnsPerHost = maxIdleConnsPerHost

	c := &Client{
		httpClient: &http.Client{Timeout: 20 * time.Second, Transport: transport},
		baseURL:    defaultBaseURL,
		token:      os.Getenv("GITHUB_TOKEN"),

		maxRetries: 6,
		retryWait:  1500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Authenticated reports whether the client will send a token.
func (c *Client) Authenticated() bool { return c.token != "" }

func (c *Client) get(ctx context.Context, path string, out any) error {
	_, err := c.getWithHeader(ctx, path, out)
	return err
}

// getWithHeader is like get but also returns the response header, needed for the Link-header pagination-count trick.
func (c *Client) getWithHeader(ctx context.Context, path string, out any) (http.Header, error) {
	for attempt := 0; ; attempt++ {
		header, body, status, err := c.doGet(ctx, path, "application/vnd.github+json")
		if err != nil {
			return nil, err
		}

		switch {
		case status == http.StatusOK:
			if out == nil {
				return header, nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return nil, fmt.Errorf("decode %s: %w", path, err)
			}
			return header, nil

		case status == http.StatusAccepted:
			if attempt >= c.maxRetries {
				return nil, fmt.Errorf("github still computing stats for %s after %d retries", path, attempt)
			}
			if err := sleep(ctx, c.retryWait); err != nil {
				return nil, err
			}
			continue

		case status == http.StatusNotFound:
			return nil, &NotFoundError{Endpoint: path}

		case isRateLimited(status, header):
			return nil, rateLimitError(header, path)

		default:
			return nil, fmt.Errorf("github %s returned %d: %s", path, status, snippet(body))
		}
	}
}

// getRaw fetches raw file bytes; the default content endpoint returns base64-encoded JSON, requiring a different Accept header.
func (c *Client) getRaw(ctx context.Context, path string) ([]byte, error) {
	header, body, status, err := c.doGet(ctx, path, "application/vnd.github.raw+json")
	if err != nil {
		return nil, err
	}
	switch {
	case status == http.StatusOK:
		return body, nil
	case status == http.StatusNotFound:
		return nil, &NotFoundError{Endpoint: path}
	case isRateLimited(status, header):
		return nil, rateLimitError(header, path)
	default:
		return nil, fmt.Errorf("github %s returned %d: %s", path, status, snippet(body))
	}
}

// doGet executes a single GET. The Close error is intentionally discarded: the
// read result is already captured, so a Close failure cannot change body or
// readErr and is not actionable.
func (c *Client) doGet(ctx context.Context, path, accept string) (http.Header, []byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("build request for %s: %w", path, err)
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, nil, 0, fmt.Errorf("request %s: %w", path, err)
	}
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, nil, 0, fmt.Errorf("read body for %s: %w", path, readErr)
	}
	return resp.Header, body, resp.StatusCode, nil
}

func isRateLimited(status int, h http.Header) bool {
	if status != http.StatusForbidden && status != http.StatusTooManyRequests {
		return false
	}
	return h.Get("X-RateLimit-Remaining") == "0"
}

func rateLimitError(h http.Header, path string) *RateLimitError {
	e := &RateLimitError{Endpoint: path, Resource: h.Get("X-RateLimit-Resource")}
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil {
		e.Reset = time.Unix(v, 0)
	}
	if v, err := strconv.Atoi(h.Get("X-RateLimit-Limit")); err == nil {
		e.Limit = v
	}
	return e
}

func snippet(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 200 {
		return s[:200] + "..."
	}
	return s
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

var errEmptyOwnerRepo = errors.New("owner and repo must both be non-empty")
