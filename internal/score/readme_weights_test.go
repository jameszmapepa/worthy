package score

import (
	"os"
	"regexp"
	"strconv"
	"testing"
)

const readmePath = "../../README.md"

func readmeLines(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatalf("read README: %v", err)
	}
	return regexp.MustCompile(`\r?\n`).Split(string(data), -1)
}

func TestReadmeCategoryWeightsMatchCode(t *testing.T) {
	want := map[string]float64{
		"Activity":  weightActivity,
		"Community": weightCommunity,
		"Security":  weightSecurity,
	}
	rowRe := regexp.MustCompile(`\*\*(Activity|Community|Security)\*\* \((\d+(?:\.\d+)?)%\)`)

	found := map[string]float64{}
	for _, line := range readmeLines(t) {
		if m := rowRe.FindStringSubmatch(line); m != nil {
			pct, err := strconv.ParseFloat(m[2], 64)
			if err != nil {
				t.Fatalf("parse %q percent: %v", m[1], err)
			}
			found[m[1]] = pct / 100
		}
	}

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

func TestReadmeCommunityWeightsMatchCode(t *testing.T) {
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
