package score

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, singular)
	}
	return fmt.Sprintf("%d %s", n, pluralForm)
}

func agoDays(days int) string {
	switch {
	case days <= 0:
		return "today"
	case days < 30:
		return fmt.Sprintf("%dd ago", days)
	case days < 730:
		return fmt.Sprintf("%dmo ago", days/30)
	default:
		return fmt.Sprintf("%.1fy ago", float64(days)/365)
	}
}

func spanDays(days int) string {
	switch {
	case days < 30:
		return plural(days, "day", "days")
	case days < 730:
		return plural(days/30, "month", "months")
	default:
		return fmt.Sprintf("%.1f years", float64(days)/365)
	}
}

func replyTime(hours float64) string {
	switch {
	case hours <= 0:
		return ""
	case hours <= 1:
		return "first reply within the hour"
	case hours < 48:
		return fmt.Sprintf("first reply in about %.0fh", hours)
	default:
		return fmt.Sprintf("first reply in about %.0fd", hours/24)
	}
}

func commitPace(raw RawMetrics) string {
	var perWeek float64
	switch {
	case len(raw.CommitsLast52Weeks) > 0:
		perWeek = medianLast(raw.CommitsLast52Weeks, 12)
	case raw.HasCommitFallback:
		perWeek = raw.CommitsPerWeekFallback
	default:
		return ""
	}
	switch {
	case perWeek == 0:
		return "no default-branch commits most weeks"
	case perWeek < 1:
		return "under 1 default-branch commit a week"
	case perWeek < 1.5:
		return "1 default-branch commit a week"
	default:
		return fmt.Sprintf("%.0f default-branch commits a week", perWeek)
	}
}

func releaseNote(raw RawMetrics) string {
	if raw.ReleaseCount == 0 {
		return "no releases"
	}
	return "last release " + agoDays(raw.DaysSinceLastRelease)
}

func newcomerMerges(raw RawMetrics) string {
	total := raw.NewcomerPRsMerged + raw.NewcomerPRsClosedUnmerged
	if total == 0 {
		return "no newcomer PRs to judge"
	}
	return fmt.Sprintf("%d of %d newcomer PRs merged", raw.NewcomerPRsMerged, total)
}

func joinFacts(facts ...string) string {
	kept := facts[:0]
	for _, f := range facts {
		if f != "" {
			kept = append(kept, f)
		}
	}
	return strings.Join(kept, ", ")
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	r, size := utf8.DecodeRuneInString(s)
	return string(unicode.ToUpper(r)) + s[size:]
}

func maintainedMessage(grade string, raw RawMetrics) string {
	facts := joinFacts("pushed "+agoDays(raw.DaysSinceLastPush), commitPace(raw), releaseNote(raw))
	switch grade {
	case "A":
		return capitalize(facts) + "."
	case "B":
		return "Active: " + facts + "."
	case "C":
		return "Mixed: " + facts + "."
	case "D":
		return "Weak: " + facts + "."
	default:
		if raw.DaysSinceLastPush <= recentPushDays {
			return "Weak: " + facts + "."
		}
		return "Dormant: " + facts + ". Assume no one will review your PR."
	}
}

const recentPushDays = 90

func contributableMessage(grade string, raw RawMetrics) string {
	stale := ""
	if raw.StaleNewcomerOpenPRs > 0 {
		stale = plural(raw.StaleNewcomerOpenPRs, "newcomer PR waiting over 30d", "newcomer PRs waiting over 30d")
	}
	guide := ""
	if raw.HasContributing {
		guide = "contributing guide present"
	}
	facts := joinFacts(newcomerMerges(raw), replyTime(raw.MedianIssueFirstResponseHours), stale, guide)
	if raw.NewcomerPRsMerged+raw.NewcomerPRsClosedUnmerged == 0 {
		return "Unproven: " + facts + ". Open an issue to test the waters before coding."
	}
	switch grade {
	case "A":
		return "Yes: " + facts + "."
	case "B":
		return "Likely: " + facts + "."
	case "C":
		return "Unclear: " + facts + ". Check the open PR queue before investing."
	case "D":
		return "Doubtful: " + facts + ". Open an issue and get a yes before coding."
	default:
		return "No: outside PRs are almost never merged (" + facts + "). Fork or move on."
	}
}

func strong(grade string) bool { return grade == "A" || grade == "B" }

func buildVerdict(maintained, contributable QuestionScore, gates []Gate, cats []CategoryScore, raw RawMetrics) string {
	evidence := capitalize(joinFacts(
		"last push "+agoDays(raw.DaysSinceLastPush),
		newcomerMerges(raw),
		replyTime(raw.MedianIssueFirstResponseHours),
	)) + "."

	var lead string
	switch {
	case criticalGate(gates) != nil:
		lead = "Look elsewhere unless you have a specific reason: " + strings.ToLower(criticalGate(gates).Title) + "."
	case maintained.Grade == "F" || contributable.Grade == "F":
		lead = "Look elsewhere unless you have a specific reason: " + weakestReason(maintained.Grade) + "."
	case strong(maintained.Grade) && strong(contributable.Grade):
		lead = "Worth your time: active and merges outside PRs."
	case strong(contributable.Grade):
		lead = "Your PR will likely land, but " + dragNote(cats, CategoryActivity, "the outlook") + "."
	case strong(maintained.Grade):
		lead = "Alive and shipping, but " + dragNote(cats, CategoryCommunity, "newcomers") + "; open an issue first."
	case maintained.Grade == "D" && contributable.Grade == "D":
		lead = "Weak on both counts: quiet and selective. Only contribute with a maintainer's nod."
	default:
		lead = "Coin flip: some activity, some newcomer merges. Check open PRs before investing."
	}
	return lead + " " + evidence
}

func criticalGate(gates []Gate) *Gate {
	for i := range gates {
		if gates[i].Severity == SeverityCritical {
			return &gates[i]
		}
	}
	return nil
}

func weakestReason(maintainedGrade string) string {
	if maintainedGrade == "F" {
		return "the project looks dormant"
	}
	return "outside PRs are almost never merged"
}

func dragNote(cats []CategoryScore, categoryKey, subject string) string {
	weak, ok := weakestIn(cats, categoryKey)
	if !ok {
		return "the " + categoryKey + " score is weak"
	}
	return strings.ToLower(weak.Label) + " drags " + subject + " (" + weak.Raw + ")"
}

func weakestIn(cats []CategoryScore, categoryKey string) (SubScore, bool) {
	for _, c := range cats {
		if c.Key != categoryKey || len(c.Subs) == 0 {
			continue
		}
		weak := c.Subs[0]
		for _, s := range c.Subs[1:] {
			if s.Value < weak.Value {
				weak = s
			}
		}
		return weak, true
	}
	return SubScore{}, false
}
