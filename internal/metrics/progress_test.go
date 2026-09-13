package metrics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type progressLog struct {
	mu     sync.Mutex
	events []Progress
}

func (l *progressLog) record(p Progress) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.events = append(l.events, p)
}

func (l *progressLog) find(stage string, state StageState) *Progress {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i := range l.events {
		if l.events[i].Stage == stage && l.events[i].State == state {
			return &l.events[i]
		}
	}
	return nil
}

func TestProgress_ReportsEveryStageRunningThenDone(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	srv := mux(countBaseRoutes(now))
	defer srv.Close()

	var log progressLog
	if _, err := Collect(context.Background(), client(srv), "acme", "widget", now, WithProgress(log.record)); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	for _, st := range Stages {
		if log.find(st, StageRunning) == nil {
			t.Errorf("stage %s never reported running", st)
		}
		if log.find(st, StageDone) == nil && log.find(st, StageDegraded) == nil {
			t.Errorf("stage %s never reported done/degraded", st)
		}
	}
	if log.events[0].Stage != StageRepository || log.events[0].State != StageRunning {
		t.Errorf("first event = %+v, want repository running", log.events[0])
	}
	repoDone := log.find(StageRepository, StageDone)
	if repoDone == nil || repoDone.Repo == nil {
		t.Fatal("repository done event must carry header metadata")
	}
	if repoDone.Repo.Stars != 1500 || repoDone.Repo.LicenseSPDX != "MIT" {
		t.Errorf("header snapshot = stars %d license %q", repoDone.Repo.Stars, repoDone.Repo.LicenseSPDX)
	}
}

func TestProgress_202RetryIsReported(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	routes := countBaseRoutes(now)
	base := mux(routes)
	defer base.Close()

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/acme/widget/stats/commit_activity" && hits.Add(1) == 1 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		r.URL.Host = base.Listener.Addr().String()
		r.URL.Scheme = "http"
		r.RequestURI = ""
		resp, err := http.DefaultClient.Do(r)
		if err != nil {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		w.WriteHeader(resp.StatusCode)
		buf := make([]byte, 1<<16)
		for {
			n, rerr := resp.Body.Read(buf)
			_, _ = w.Write(buf[:n])
			if rerr != nil {
				break
			}
		}
	}))
	defer srv.Close()

	var log progressLog
	raw, err := Collect(context.Background(), client(srv), "acme", "widget", now, WithProgress(log.record))
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	retry := log.find(StageCommits, StageRetrying)
	if retry == nil {
		t.Fatal("expected a retrying event for commit_activity after a 202")
	}
	if retry.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", retry.Attempt)
	}
	if containsStr(raw.Partial, "commit_activity") {
		t.Error("commit activity should succeed after the retry, not degrade")
	}
	if log.find(StageCommits, StageDone) == nil {
		t.Error("commit_activity should report done after the retry")
	}
}

func TestProgress_DegradedStageReported(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	routes := countBaseRoutes(now)
	routes["/repos/acme/widget/community/profile"] = fixture{status: http.StatusNotFound, body: `{}`}
	srv := mux(routes)
	defer srv.Close()

	var log progressLog
	if _, err := Collect(context.Background(), client(srv), "acme", "widget", now, WithProgress(log.record)); err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if log.find(StageCommunity, StageDegraded) == nil {
		t.Error("community profile 404 should report the stage as degraded")
	}
}

func TestProgress_NilCallbackIsSafe(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-06-22T00:00:00Z")
	srv := mux(countBaseRoutes(now))
	defer srv.Close()
	if _, err := Collect(context.Background(), client(srv), "acme", "widget", now); err != nil {
		t.Fatalf("Collect without progress: %v", err)
	}
}
