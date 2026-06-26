package tui

import (
	"fmt"
	"image/color"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/worthy/internal/score"
)

const (
	glyphStar    = "★"
	glyphFork    = "⑂"
	glyphWatcher = "◉"
)

var headerPanelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorBorder).
	Padding(0, 1)

func renderHeaderPanel(owner, repo string, raw score.RawMetrics, loaded, authenticated bool, width int, grade string, ascii bool) string {
	boxW := clampWidth(width-2, 24, 200)
	textW := boxW - 2

	identity := titleStyle.Render(owner + "/" + repo)

	if grade != "" {
		identity += " " + mutedStyle.Render("·") + " " +
			gradeStyle.Render("Grade "+grade)
	}

	badge := rateLimitBadge(authenticated)
	top := joinEnds(identity, badge, textW)

	rows := []string{top}
	if loaded {
		if desc := strings.TrimSpace(raw.Description); desc != "" {
			rows = append(rows, mutedStyle.Render(truncate(desc, textW)))
		}
		rows = append(rows, metaRow(raw, ascii))
	}

	body := strings.Join(rows, "\n")
	return headerPanelStyle.Width(boxW).Render(body)
}

func metaRow(raw score.RawMetrics, ascii bool) string {
	stat := func(glyph string, glyphColor color.Color, n int) string {
		icon := lipgloss.NewStyle().Foreground(glyphColor).Render(glyph)
		return icon + " " + labelStyle.Render(humanizeCount(n))
	}
	parts := []string{
		stat(glyphStar, colorStar, raw.Stars),
		stat(glyphFork, colorFork, raw.Forks),
		stat(glyphWatcher, colorWatcher, raw.Watchers),
	}
	if raw.Language != "" {
		parts = append(parts, languageBadge(raw.Language, ascii))
	}
	parts = append(parts, mutedStyle.Render(licenseLabel(raw.LicenseSPDX)))
	parts = append(parts, mutedStyle.Render(humanizeAge(raw.RepoAgeDays)))
	return strings.Join(parts, mutedStyle.Render("   "))
}

func rateLimitBadge(authenticated bool) string {
	rate, rateColor := "60/hr", colorAmber
	if authenticated {
		rate, rateColor = "5,000/hr", colorGreen
	}
	return mutedStyle.Render("API ") + lipgloss.NewStyle().Foreground(rateColor).Render(rate)
}

func licenseLabel(spdx string) string {
	if spdx == "" || spdx == "NOASSERTION" {
		return "no license"
	}
	return spdx
}

func humanizeCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func humanizeAge(days int) string {
	switch {
	case days <= 0:
		return "new"
	case days < 30:
		return fmt.Sprintf("%dd old", days)
	case days < 365:
		return fmt.Sprintf("%.1fmo old", float64(days)/30)
	default:
		return fmt.Sprintf("%.1fy old", float64(days)/365)
	}
}

func joinEnds(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}
