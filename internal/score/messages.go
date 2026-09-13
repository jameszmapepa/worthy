package score

import (
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

func itoa(n int) string { return strconv.Itoa(n) }

func ftoa(f float64, decimals int) string { return strconv.FormatFloat(f, 'f', decimals, 64) }

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return itoa(n) + " " + singular
	}
	return itoa(n) + " " + pluralForm
}

func agoDays(days int) string {
	switch {
	case days <= 0:
		return "today"
	case days < 30:
		return itoa(days) + "d ago"
	case days < 730:
		return itoa(days/30) + "mo ago"
	default:
		return ftoa(float64(days)/365, 1) + "y ago"
	}
}

func spanDays(days int) string {
	switch {
	case days < 30:
		return plural(days, "day", "days")
	case days < 730:
		return plural(days/30, "month", "months")
	default:
		return ftoa(float64(days)/365, 1) + " years"
	}
}

func replyTime(hours float64) string {
	switch {
	case hours <= 0:
		return ""
	case hours <= 1:
		return "first reply within the hour"
	case hours < 48:
		return "first reply in about " + ftoa(hours, 0) + "h"
	default:
		return "first reply in about " + ftoa(hours/24, 0) + "d"
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
		return ftoa(perWeek, 0) + " default-branch commits a week"
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
	return itoa(raw.NewcomerPRsMerged) + " of " + itoa(total) + " newcomer PRs merged"
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

type evidence struct {
	pushAgo string
	merges  string
	reply   string
}

func gatherEvidence(raw RawMetrics) evidence {
	return evidence{
		pushAgo: agoDays(raw.DaysSinceLastPush),
		merges:  newcomerMerges(raw),
		reply:   replyTime(raw.MedianIssueFirstResponseHours),
	}
}

func maintainedMessage(grade string, raw RawMetrics) string {
	return maintainedMessageWith(grade, raw, gatherEvidence(raw))
}

func maintainedMessageWith(grade string, raw RawMetrics, ev evidence) string {
	facts := joinFacts("pushed "+ev.pushAgo, commitPace(raw), releaseNote(raw))
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
	return contributableMessageWith(grade, raw, gatherEvidence(raw))
}

func contributableMessageWith(grade string, raw RawMetrics, ev evidence) string {
	stale := ""
	if raw.StaleNewcomerOpenPRs > 0 {
		stale = plural(raw.StaleNewcomerOpenPRs, "newcomer PR waiting over 30d", "newcomer PRs waiting over 30d")
	}
	guide := ""
	if raw.HasContributing {
		guide = "contributing guide present"
	}
	facts := joinFacts(ev.merges, ev.reply, stale, guide)
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
	return buildVerdictWith(maintained, contributable, gates, cats, gatherEvidence(raw))
}

func buildVerdictWith(maintained, contributable QuestionScore, gates []Gate, cats []CategoryScore, ev evidence) string {
	evidence := capitalize(joinFacts("last push "+ev.pushAgo, ev.merges, ev.reply)) + "."

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
