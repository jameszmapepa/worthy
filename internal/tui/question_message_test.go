package tui

import (
	"strings"
	"testing"
)

func TestQuestionCardsShowMessages(t *testing.T) {
	r := fixedReport()
	out := renderScorecard(r, 100, -1, false)
	for _, msg := range []string{r.Maintained.Message, r.Contributable.Message} {
		if msg == "" || !strings.Contains(out, msg[:min(len(msg), 24)]) {
			t.Errorf("scorecard should show question message %q in:\n%s", msg, out)
		}
	}
}
