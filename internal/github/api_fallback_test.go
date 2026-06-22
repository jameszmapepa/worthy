package github

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCommitCountSince_LinkHeaderCount(t *testing.T) {
	var gotSince string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotSince = r.URL.Query().Get("since")
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Link",
			`<https://api/repos/o/r/commits?per_page=1&page=2>; rel="next", `+
				`<https://api/repos/o/r/commits?per_page=1&page=137>; rel="last"`)
		_, _ = w.Write([]byte(`[{"sha":"abc"}]`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	since := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	n, err := c.CommitCountSince(context.Background(), "o", "r", since)
	if err != nil {
		t.Fatalf("CommitCountSince: %v", err)
	}
	if n != 137 {
		t.Errorf("count = %d; want 137 (rel=last page)", n)
	}
	if gotSince == "" {
		t.Error("since query param was not sent")
	}
}

func TestCommitCountSince_NoLinkHeader(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want int
	}{
		{"one commit", `[{"sha":"abc"}]`, 1},
		{"no commits", `[]`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			c := newTestClient(srv)
			n, err := c.CommitCountSince(context.Background(), "o", "r", time.Now())
			if err != nil {
				t.Fatalf("CommitCountSince: %v", err)
			}
			if n != tc.want {
				t.Errorf("count = %d; want %d", n, tc.want)
			}
		})
	}
}

func TestSearchIssueCount(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"total_count":42,"items":[]}`))
	}))
	defer srv.Close()

	c := newTestClient(srv)
	n, err := c.SearchIssueCount(context.Background(), `repo:o/r is:issue label:"good first issue"`)
	if err != nil {
		t.Fatalf("SearchIssueCount: %v", err)
	}
	if n != 42 {
		t.Errorf("count = %d; want 42", n)
	}
	if gotQuery == "" {
		t.Error("q query param was not forwarded")
	}
}

func TestLastPageFromLink(t *testing.T) {
	for _, tc := range []struct {
		name string
		link string
		want int
	}{
		{"rel=last present", `<https://x?page=5>; rel="next", <https://x?page=99>; rel="last"`, 99},
		{"no last rel", `<https://x?page=2>; rel="next"`, 0},
		{"empty", "", 0},
		{"last with trailing params", `<https://x?per_page=1&page=12&state=open>; rel="last"`, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.link != "" {
				h.Set("Link", tc.link)
			}
			if got := lastPageFromLink(h); got != tc.want {
				t.Errorf("lastPageFromLink(%q) = %d; want %d", tc.link, got, tc.want)
			}
		})
	}
}
