package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultBaseURL = "https://api.github.com"
	apiVersion     = "2022-11-28"
	userAgent      = "repo-health (https://github.com/jameszmapepa/repo-health)"

	// maxIdleConnsPerHost sizes the connection pool for the single host we
	// talk to. The default Transport caps this at 2, which serialises
	// concurrent calls onto two sockets and defeats the bounded worker pool.
	// ceiling: should track metrics.maxConcurrency (8). We cannot import that
	// package without a cycle, so the value is duplicated here intentionally;
	// keep them in sync if the concurrency bound changes.
	maxIdleConnsPerHost = 8
)

// RateLimitError is returned when the GitHub API rejects a request because the
// rate limit is exhausted. Reset reports when the window refreshes.
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

// NotFoundError is returned for a 404 (repo missing, private, or endpoint
// unavailable such as community/profile on a fork).
type NotFoundError struct{ Endpoint string }

func (e *NotFoundError) Error() string { return fmt.Sprintf("not found: %s", e.Endpoint) }

// Client is a minimal GitHub REST client. The zero value is not usable; call
// NewClient. It is safe for concurrent use.
type Client struct {
	httpClient *http.Client
	baseURL    string
	token      string
	maxRetries int           // for 202 (stats recompute) responses
	retryWait  time.Duration // base wait between 202 retries
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

// NewClient builds a Client. By default it is unauthenticated; if a token was
// not supplied via WithToken it falls back to the GITHUB_TOKEN environment
// variable when present. No token is ever required.
func NewClient(opts ...Option) *Client {
	// Clone the default transport so we can size its per-host pool without
	// mutating the shared global. Guard the assertion: if a caller has
	// replaced http.DefaultTransport with a non-*http.Transport, fall back to
	// a fresh one rather than panicking at startup.
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
		maxRetries: 3,
		retryWait:  1500 * time.Millisecond,
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Authenticated reports whether the client will send a token. Used only to
// inform the user of their effective rate-limit ceiling.
func (c *Client) Authenticated() bool { return c.token != "" }

// get performs a GET against path (e.g. "/repos/o/r"), decoding the JSON body
// into out. It transparently retries 202 (stats recompute) responses and
// returns a typed RateLimitError / NotFoundError where applicable.
func (c *Client) get(ctx context.Context, path string, out any) error {
	_, err := c.getWithHeader(ctx, path, out)
	return err
}

// getWithHeader is like get but also returns the response header (needed for
// the Link-header pagination-count trick).
func (c *Client) getWithHeader(ctx context.Context, path string, out any) (http.Header, error) {
	endpoint := c.baseURL + path
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("build request for %s: %w", path, err)
		}
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", apiVersion)
		req.Header.Set("User-Agent", userAgent)
		if c.token != "" {
			req.Header.Set("Authorization", "Bearer "+c.token)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request %s: %w", path, err)
		}

		header, body, err := drain(resp)
		if err != nil {
			return nil, fmt.Errorf("read body for %s: %w", path, err)
		}

		switch {
		case resp.StatusCode == http.StatusOK:
			if out == nil {
				return header, nil
			}
			if err := json.Unmarshal(body, out); err != nil {
				return nil, fmt.Errorf("decode %s: %w", path, err)
			}
			return header, nil

		case resp.StatusCode == http.StatusAccepted:
			// Stats endpoints return 202 while GitHub computes the result in the
			// background. Retry a bounded number of times, then give up cleanly.
			if attempt >= c.maxRetries {
				return nil, fmt.Errorf("github still computing stats for %s after %d retries", path, attempt)
			}
			if err := sleep(ctx, c.retryWait*time.Duration(attempt+1)); err != nil {
				return nil, err
			}
			continue

		case resp.StatusCode == http.StatusNotFound:
			return nil, &NotFoundError{Endpoint: path}

		case isRateLimited(resp.StatusCode, header):
			return nil, rateLimitError(header, path)

		default:
			return nil, fmt.Errorf("github %s returned %d: %s", path, resp.StatusCode, snippet(body))
		}
	}
}

// getRaw performs a GET that returns raw file bytes (Accept: raw+json).
// Used for workflow file contents where we need plain text, not base64 JSON.
func (c *Client) getRaw(ctx context.Context, path string) ([]byte, error) {
	endpoint := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build request for %s: %w", path, err)
	}
	req.Header.Set("Accept", "application/vnd.github.raw+json")
	req.Header.Set("X-GitHub-Api-Version", apiVersion)
	req.Header.Set("User-Agent", userAgent)
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request %s: %w", path, err)
	}
	_, body, err := drain(resp)
	if err != nil {
		return nil, fmt.Errorf("read body for %s: %w", path, err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return body, nil
	case http.StatusNotFound:
		return nil, &NotFoundError{Endpoint: path}
	default:
		return nil, fmt.Errorf("github %s returned %d: %s", path, resp.StatusCode, snippet(body))
	}
}

func drain(resp *http.Response) (http.Header, []byte, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20)) // 16 MiB cap
	return resp.Header, body, err
}

// isRateLimited detects GitHub's rate-limit rejection: a 403 or 429 with the
// remaining-requests header at zero.
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

// sleep waits d or returns ctx.Err() if the context is cancelled first.
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

// lastPageCount extracts the total item count for a paginated collection by
// reading the rel="last" page number from the Link header of a per_page=1
// request. When there is no Link header (a single page) the caller falls back
// to counting the returned slice.
func lastPageCount(h http.Header) (int, bool) {
	link := h.Get("Link")
	if link == "" {
		return 0, false
	}
	for _, part := range strings.Split(link, ",") {
		if !strings.Contains(part, `rel="last"`) {
			continue
		}
		start := strings.Index(part, "<")
		end := strings.Index(part, ">")
		if start < 0 || end < 0 || end <= start {
			continue
		}
		u, err := url.Parse(part[start+1 : end])
		if err != nil {
			continue
		}
		if p := u.Query().Get("page"); p != "" {
			if n, err := strconv.Atoi(p); err == nil {
				return n, true
			}
		}
	}
	return 0, false
}

var errEmptyOwnerRepo = errors.New("owner and repo must both be non-empty")
