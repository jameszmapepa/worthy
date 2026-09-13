package github

import (
	"context"
	"net/http"
	"net/http/httptest"
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
