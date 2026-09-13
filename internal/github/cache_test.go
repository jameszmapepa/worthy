package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func etagServer(t *testing.T, body string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("X-RateLimit-Remaining", "41")
		w.Header().Set("X-RateLimit-Limit", "60")
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Link", `<https://x/?page=7>; rel="last"`)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func TestCache_FreshEntryServedWithoutNetwork(t *testing.T) {
	srv, hits := etagServer(t, `{"n":1}`)
	c := newTestClient(srv, WithCache(t.TempDir(), time.Hour))

	var out struct{ N int }
	for range 3 {
		if err := c.get(context.Background(), "/thing", &out); err != nil {
			t.Fatal(err)
		}
	}
	if out.N != 1 {
		t.Errorf("decoded n = %d", out.N)
	}
	if got := hits.Load(); got != 1 {
		t.Errorf("server hits = %d, want 1 (later calls served from cache)", got)
	}
}

func TestCache_StaleEntryRevalidatesAndReusesBodyOn304(t *testing.T) {
	srv, hits := etagServer(t, `{"n":2}`)
	c := newTestClient(srv, WithCache(t.TempDir(), 0))

	var out struct{ N int }
	if err := c.get(context.Background(), "/thing", &out); err != nil {
		t.Fatal(err)
	}
	out.N = 0
	hdr, err := c.getWithHeader(context.Background(), "/thing", &out)
	if err != nil {
		t.Fatal(err)
	}
	if out.N != 2 {
		t.Errorf("body after 304 = %d, want 2 from cache", out.N)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2 (one full, one 304)", got)
	}
	if lastPageFromLink(hdr) != 7 {
		t.Errorf("Link header must survive the cache; got %q", hdr.Get("Link"))
	}
}

func TestCache_ForceRevalidateSkipsFreshness(t *testing.T) {
	srv, hits := etagServer(t, `{}`)
	c := newTestClient(srv, WithCache(t.TempDir(), time.Hour))
	ctx := context.Background()
	if err := c.get(ctx, "/thing", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.get(ForceRevalidate(ctx), "/thing", nil); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("server hits = %d, want 2 (forced revalidation)", got)
	}
}

func TestCache_NonOKResponsesAreNotStored(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := hits.Add(1)
		w.Header().Set("ETag", `"x"`)
		if n == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newTestClient(srv, WithCache(t.TempDir(), time.Hour))
	if err := c.get(context.Background(), "/stats", nil); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("hits = %d, want 2 (202 must not be cached and retried from disk)", got)
	}
}

func TestCache_RawAndJSONAcceptAreSeparateEntries(t *testing.T) {
	srv, hits := etagServer(t, `raw-bytes`)
	c := newTestClient(srv, WithCache(t.TempDir(), time.Hour))
	if _, err := c.getRaw(context.Background(), "/f"); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := c.doGet(context.Background(), "/f", "application/vnd.github+json"); err != nil {
		t.Fatal(err)
	}
	if got := hits.Load(); got != 2 {
		t.Errorf("hits = %d, want 2 (different Accept headers must not share an entry)", got)
	}
}

func TestCache_UnwritableDirIsHarmless(t *testing.T) {
	srv, _ := etagServer(t, `{}`)
	c := newTestClient(srv, WithCache("/dev/null/nope", time.Hour))
	if err := c.get(context.Background(), "/thing", nil); err != nil {
		t.Fatalf("cache failures must not surface: %v", err)
	}
}

func TestRateInfo_TracksHeaders(t *testing.T) {
	srv, _ := etagServer(t, `{}`)
	c := newTestClient(srv, WithCache(t.TempDir(), time.Hour))
	if c.RateInfo().Known {
		t.Fatal("rate info must be unknown before any request")
	}
	if err := c.get(context.Background(), "/thing", nil); err != nil {
		t.Fatal(err)
	}
	got := c.RateInfo()
	if !got.Known || got.Remaining != 41 || got.Limit != 60 {
		t.Errorf("rate info = %+v", got)
	}
	if err := c.get(context.Background(), "/thing", nil); err != nil {
		t.Fatal(err)
	}
	if c.RateInfo() != got {
		t.Errorf("rate info changed on cache hit: %+v", c.RateInfo())
	}
}

func TestRateInfo_IgnoresNonCoreResources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "26")
		w.Header().Set("X-RateLimit-Limit", "30")
		w.Header().Set("X-RateLimit-Resource", "search")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	if err := c.get(context.Background(), "/search/issues", nil); err != nil {
		t.Fatal(err)
	}
	if c.RateInfo().Known {
		t.Errorf("search budget must not be reported as the core budget: %+v", c.RateInfo())
	}
}

func TestDefaultCacheDir_EndsWithWorthy(t *testing.T) {
	dir, err := DefaultCacheDir()
	if err != nil {
		t.Skip("no user cache dir on this platform")
	}
	if dir == "" || dir[len(dir)-6:] != "worthy" {
		t.Errorf("cache dir = %q", dir)
	}
}

func TestServerError_RetriedThenSucceeds(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusGatewayTimeout)
			_, _ = w.Write([]byte("<!DOCTYPE html><!-- Hello future GitHubber! -->"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	if err := newTestClient(srv).get(context.Background(), "/repos/a/b", nil); err != nil {
		t.Fatalf("two 504s then 200 should succeed, got %v", err)
	}
	if hits.Load() != 3 {
		t.Errorf("hits = %d, want 3", hits.Load())
	}
}

func TestServerError_ExhaustedIsTypedWithoutHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
		_, _ = w.Write([]byte("<!DOCTYPE html><!-- Hello future GitHubber! -->"))
	}))
	defer srv.Close()
	c := newTestClient(srv)
	err := c.get(context.Background(), "/repos/a/b", nil)
	var se *ServerError
	if !errors.As(err, &se) || se.Status != http.StatusGatewayTimeout {
		t.Fatalf("want *ServerError 504, got %T %v", err, err)
	}
	if strings.Contains(err.Error(), "DOCTYPE") || strings.Contains(err.Error(), "GitHubber") {
		t.Errorf("error must not echo the HTML page: %v", err)
	}
	if _, rerr := c.getRaw(context.Background(), "/repos/a/b/contents/x"); !errors.As(rerr, &se) {
		t.Errorf("getRaw should return *ServerError too, got %T", rerr)
	}
}

func TestSnippetHidesHTML(t *testing.T) {
	if got := snippet([]byte("<!DOCTYPE html><html>")); got != "(html error page)" {
		t.Errorf("snippet = %q", got)
	}
	if got := snippet([]byte(`{"message":"bad"}`)); got != `{"message":"bad"}` {
		t.Errorf("snippet = %q", got)
	}
}
