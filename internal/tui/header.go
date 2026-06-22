package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jameszmapepa/repo-health/internal/score"
)

// Meta-row glyphs.
const (
	glyphStar    = "★"
	glyphFork    = "⑂"
	glyphWatcher = "◉"
)

// headerPanelStyle wraps the header in a rounded border with the accent color.
var headerPanelStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(colorBorder).
	Padding(0, 1)

// renderHeaderPanel builds the bordered header shown on every view. owner/repo
// is always present; the description and meta row appear once metrics are
// loaded (loaded reports whether raw has been populated).
func renderHeaderPanel(owner, repo string, raw score.RawMetrics, loaded, authenticated bool, width int) string {
	// lipgloss Style.Width(w) sets the box width INCLUDING padding; the border
	// adds 2 more cells outside it. So for a panel of total terminal width
	// `width`, the box is width-2 and the usable text inside is width-2-2.
	boxW := clampWidth(width-2, 24, 200)
	textW := boxW - 2 // minus left+right padding

	identity := titleStyle.Render(owner + "/" + repo)
	badge := rateLimitBadge(authenticated)
	top := joinEnds(identity, badge, textW)

	rows := []string{top}
	if loaded {
		if desc := strings.TrimSpace(raw.Description); desc != "" {
			rows = append(rows, mutedStyle.Render(truncate(desc, textW)))
		}
		rows = append(rows, metaRow(raw))
	}

	body := strings.Join(rows, "\n")
	return headerPanelStyle.Width(boxW).Render(body)
}

// metaRow renders the star/fork/watcher counts, language, license, and age.
func metaRow(raw score.RawMetrics) string {
	parts := []string{
		fmt.Sprintf("%s %s", glyphStar, humanizeCount(raw.Stars)),
		fmt.Sprintf("%s %s", glyphFork, humanizeCount(raw.Forks)),
		fmt.Sprintf("%s %s", glyphWatcher, humanizeCount(raw.Watchers)),
	}
	if raw.Language != "" {
		parts = append(parts, raw.Language)
	}
	parts = append(parts, licenseLabel(raw.LicenseSPDX))
	parts = append(parts, humanizeAge(raw.RepoAgeDays))
	return mutedStyle.Render(strings.Join(parts, "   "))
}

// rateLimitBadge renders a color-coded rate-limit indicator.
func rateLimitBadge(authenticated bool) string {
	if authenticated {
		return lipgloss.NewStyle().Foreground(colorGreen).Render("5,000/hr")
	}
	return lipgloss.NewStyle().Foreground(colorAmber).Render("60/hr")
}

// licenseLabel returns a display label for a license SPDX id.
func licenseLabel(spdx string) string {
	if spdx == "" || spdx == "NOASSERTION" {
		return "no license"
	}
	return spdx
}

// humanizeCount abbreviates large counts (e.g. 4200 -> "4.2k").
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

// humanizeAge renders a repo age in days as a compact human string.
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

// joinEnds places left and right on one line of total width, padding the gap
// between them. If they do not fit, they are simply separated by a space.
func joinEnds(left, right string, width int) string {
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return left + " " + right
	}
	return left + strings.Repeat(" ", gap) + right
}
