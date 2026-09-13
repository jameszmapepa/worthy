package tui

import (
	"strings"

	"github.com/jameszmapepa/worthy/internal/github"
	"github.com/jameszmapepa/worthy/internal/score"
)

// PlainOptions configures RenderPlain.
type PlainOptions struct {
	Width         int
	ASCIIIcons    bool
	Authenticated bool
	Rate          github.RateInfo
}

// RenderPlain renders the scorecard and explanation as one static page for
// pipes, CI logs and the --plain flag. Styling is left in; the caller decides
// whether to strip it based on the destination.
func RenderPlain(owner, repo string, r score.Report, raw score.RawMetrics, opts PlainOptions) string {
	width := opts.Width
	if width <= 0 {
		width = 100
	}
	var b strings.Builder
	b.WriteString(renderHeaderPanel(owner, repo, raw, true, opts.Authenticated, opts.Rate, width, r.Grade, opts.ASCIIIcons))
	b.WriteString("\n\n")
	b.WriteString(renderScorecard(r, width, -1, false))
	b.WriteString("\n")
	b.WriteString(renderExplain(r, width))
	b.WriteString("\n")
	return b.String()
}
