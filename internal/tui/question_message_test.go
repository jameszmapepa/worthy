package tui

import (
	"strings"
	"testing"
)

// TestQuestionCardsShowMessages verifies the two headline cards render their
// plain-language per-question reading, not just a bare letter grade.
func TestQuestionCardsShowMessages(t *testing.T) {
	r := fixedReport()
	out := renderScorecard(r, 100, -1, false)
	for _, msg := range []string{r.Maintained.Message, r.Contributable.Message} {
		if msg == "" || !strings.Contains(out, msg) {
			t.Errorf("scorecard should show question message %q in:\n%s", msg, out)
		}
	}
}
