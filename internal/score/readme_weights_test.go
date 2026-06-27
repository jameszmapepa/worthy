package score

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

// The README's "scoring model" table documents the category weights and the
// per-sub Community weights as literal numbers. Those numbers are hand-written
// prose with no compiler link to score.go, so they silently drift the moment a
// weight changes in code. These tests pin the documented numbers to the weights
// Evaluate actually produces, failing in EITHER direction (code changed but doc
// stale, or doc edited wrong). When a weight legitimately changes, update both
// the constant/withWeight call and the README row — the test enforces that pair.

const readmePath = "../../README.md"

func readmeLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	return regexp.MustCompile(`\r?\n`).Split(string(data), -1)
}

// TestReadmeCategoryWeightsMatchCode pins the "**Activity** (45%)" style
// percentages in the scoring-model table to the weight* constants.
func TestReadmeCategoryWeightsMatchCode(t *testing.T) {
	// Arrange
	want := map[string]float64{
		"Activity":  weightActivity,
		"Community": weightCommunity,
		"Security":  weightSecurity,
	}
	rowRe := regexp.MustCompile(`\*\*(Activity|Community|Security)\*\* \((\d+)%\)`)

	// Act
	found := map[string]float64{}
	for _, line := range readmeLines(t) {
		if m := rowRe.FindStringSubmatch(line); m != nil {
			pct, err := strconv.Atoi(m[2])
			if err != nil {
				t.Fatalf("parse %q percent: %v", m[1], err)
			}
			found[m[1]] = float64(pct) / 100
		}
	}

	// Assert
	if len(found) != len(want) {
		t.Fatalf("documented %d category weights, want %d (rows: %v)", len(found), len(want), found)
	}
	for name, w := range want {
		if got, ok := found[name]; !ok {
			t.Errorf("README does not document a %s category weight", name)
		} else if !floatEq(got, w) {
			t.Errorf("README %s weight = %.2f, code = %.2f", name, got, w)
		}
	}
}

// TestReadmeCommunityWeightsMatchCode pins the ordered "(.NN)" weights in the
// Community row to the per-sub weights Evaluate assigns to the Community
// category, catching count drift (a sub added/removed) and value drift.
func TestReadmeCommunityWeightsMatchCode(t *testing.T) {
	// Arrange: the weights the scorer actually applies, in declared order.
	var community CategoryScore
	for _, c := range Evaluate(healthyRaw()).Categories {
		if c.Key == CategoryCommunity {
			community = c
		}
	}
	codeWeights := make([]float64, 0, len(community.Subs))
	for _, s := range community.Subs {
		codeWeights = append(codeWeights, s.Weight)
	}
	if len(codeWeights) == 0 {
		t.Fatal("no Community sub-scores found in report")
	}

	// Act: pull the ordered (.NN) literals out of the Community table row.
	var docWeights []float64
	weightRe := regexp.MustCompile(`\((\.\d+)\)`)
	for _, line := range readmeLines(t) {
		if !regexp.MustCompile(`\*\*Community\*\*`).MatchString(line) {
			continue
		}
		for _, m := range weightRe.FindAllStringSubmatch(line, -1) {
			w, err := strconv.ParseFloat("0"+m[1], 64)
			if err != nil {
				t.Fatalf("parse weight %q: %v", m[1], err)
			}
			docWeights = append(docWeights, w)
		}
	}

	// Assert: same count and same ordered values.
	if len(docWeights) != len(codeWeights) {
		t.Fatalf("README documents %d Community weights %v, code has %d %v",
			len(docWeights), docWeights, len(codeWeights), codeWeights)
	}
	for i := range codeWeights {
		if !floatEq(docWeights[i], codeWeights[i]) {
			t.Errorf("Community weight #%d (%s): README %.2f, code %.2f",
				i+1, community.Subs[i].Key, docWeights[i], codeWeights[i])
		}
	}
}

func floatEq(a, b float64) bool {
	const tol = 1e-9
	d := a - b
	return d < tol && d > -tol
}
