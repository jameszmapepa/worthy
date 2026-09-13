package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jameszmapepa/worthy/internal/score"
)

func sampleReport() (score.Report, score.RawMetrics) {
	raw := score.RawMetrics{
		CommitsLast52Weeks: make([]int, 52), DaysSinceLastPush: 3, RepoAgeDays: 900,
		Stars: 4200, Forks: 310, Watchers: 120, Description: "A TUI framework", Language: "Go", LicenseSPDX: "MIT",
		ContributorCount: 40, MergedPRs: 80, ClosedUnmergedPRs: 20, ReleaseCount: 12, DaysSinceLastRelease: 20,
		HasReadme: true, HasContributing: true, HasCI: true, WorkflowsFetched: true,
	}
	for i := range raw.CommitsLast52Weeks {
		raw.CommitsLast52Weeks[i] = 10
	}
	return score.Evaluate(raw), raw
}

func TestJSONShape(t *testing.T) {
	r, raw := sampleReport()
	var buf bytes.Buffer
	if err := JSON(&buf, "charmbracelet", "bubbletea", r, raw); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	for _, key := range []string{"repository", "url", "grade", "score", "verdict", "confidence", "questions", "categories", "gates", "partial_metrics", "repo"} {
		if _, ok := got[key]; !ok {
			t.Errorf("missing top-level key %q", key)
		}
	}
	if got["repository"] != "charmbracelet/bubbletea" || got["grade"] != r.Grade {
		t.Errorf("repository/grade = %v / %v", got["repository"], got["grade"])
	}
	if got["confidence"] != strings.ToLower(r.Confidence.String()) {
		t.Errorf("confidence = %v", got["confidence"])
	}
	cats := got["categories"].([]any)
	if len(cats) != 3 {
		t.Fatalf("categories = %d, want 3", len(cats))
	}
	first := cats[0].(map[string]any)
	inds := first["indicators"].([]any)
	if len(inds) == 0 {
		t.Fatal("first category has no indicators")
	}
	ind := inds[0].(map[string]any)
	for _, key := range []string{"key", "label", "score", "grade", "raw", "formula", "weight"} {
		if _, ok := ind[key]; !ok {
			t.Errorf("indicator missing %q", key)
		}
	}
	if !strings.HasSuffix(buf.String(), "\n") {
		t.Error("output should end with a newline")
	}
}

func TestBuildNeverEmitsNullSlices(t *testing.T) {
	d := Build("o", "r", score.Report{}, score.RawMetrics{})
	b, _ := json.Marshal(d)
	if strings.Contains(string(b), "null") {
		t.Errorf("empty report should encode empty arrays, not null: %s", b)
	}
}
