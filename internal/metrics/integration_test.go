package metrics

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// integrationNow is a fixed reference time so the wired fixtures are
// deterministic across runs.
var integrationNow = time.Date(2026, 6, 22, 0, 0, 0, 0, time.UTC)

// TestIntegration_CollectToEvaluate wires the real collection pipeline to the
// real scorer over loopback HTTP: httptest server -> Collect -> score.Evaluate.
// It is a Medium integration test (loopback I/O, no external network) that
// guards the metrics->score contract end to end — the field names, units, and
// Partial wiring the two packages silently agree on. A unit test on either side
// cannot catch a drift where Collect populates a field the scorer reads under a
// different name; this test can.
func TestIntegration_CollectToEvaluate(t *testing.T) {
	srv := httptest.NewServer(fullRoutesHandler(integrationNow, 0))
	defer srv.Close()

	raw, err := Collect(context.Background(), client(srv), "acme", "widget", integrationNow)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	// Every endpoint served valid data, so nothing should have degraded.
	if len(raw.Partial) != 0 {
		t.Errorf("raw.Partial = %v; want empty on the all-healthy fixture", raw.Partial)
	}

	report := score.Evaluate(raw)

	if report.Grade == "" {
		t.Error("report.Grade is empty; Evaluate produced no grade")
	}
	if report.Composite <= 0 {
		t.Errorf("report.Composite = %.1f; want > 0", report.Composite)
	}
	if report.AdjustedComposite <= 0 {
		t.Errorf("report.AdjustedComposite = %.1f; want > 0", report.AdjustedComposite)
	}
	if report.AdjustedComposite > report.Composite {
		t.Errorf("AdjustedComposite (%.1f) > Composite (%.1f); gate caps can only lower the score",
			report.AdjustedComposite, report.Composite)
	}
	if len(report.Categories) != 3 {
		t.Errorf("len(report.Categories) = %d; want 3 (activity, community, security)", len(report.Categories))
	}
	if report.Verdict == "" {
		t.Error("report.Verdict is empty; Evaluate produced no plain-language summary")
	}
}
