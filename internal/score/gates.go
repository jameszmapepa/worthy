package score

import (
	"fmt"
	"math"
)

// Severity constants classify how serious a triggered gate's condition is.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)

var (
	capBusFactor = 70.0
	capStrangers = 75.0
	capArchived  = 40.0
	capStale     = 60.0
	capIntegrity = 80.0
)

// Gate is a conditional annotation on a Report; a non-nil CapTo caps the adjusted composite score.
type Gate struct {
	Key        string
	Severity   string
	Title      string
	Detail     string
	HowToClear string
	CapTo      *float64
}

type subLookup struct {
	issueCloseRatio   float64
	prAcceptance      float64
	newcomerMergeRate float64
}

func evaluateGates(raw RawMetrics, rawComposite float64, subs subLookup) []Gate {
	var gates []Gate

	if g, ok := busFactorGate(raw); ok {
		gates = append(gates, g)
	}
	if g, ok := closedToStrangersGate(raw, subs); ok {
		gates = append(gates, g)
	}
	if g, ok := staleOrArchivedGate(raw, subs); ok {
		gates = append(gates, g)
	}
	if g, ok := integrityRiskGate(raw, rawComposite); ok {
		gates = append(gates, g)
	}
	if g, ok := vanityStarsGate(raw); ok {
		gates = append(gates, g)
	}
	return gates
}

// busFactorGateThreshold is the contributor-count ceiling for the bus_factor gate; raised from 2 to 4 because ≤2 was bypassable via two throwaway alt-account commits.
const busFactorGateThreshold = 4

func busFactorGate(raw RawMetrics) (Gate, bool) {
	if raw.TopContributorRecentShare > 0.80 && raw.ContributorCount <= busFactorGateThreshold {
		return Gate{
			Key:      "bus_factor",
			Severity: SeverityWarn,
			Title:    "Bus factor risk",
			Detail: fmt.Sprintf("One person writes %.1f%% of recent commits and only %s. If they step away, your merged work may go unmaintained.",
				raw.TopContributorRecentShare*100, plural(raw.ContributorCount, "person contributes", "people contribute")),
			HowToClear: "Distribute commits beyond the top author and grow the contributor base.",
			CapTo:      ptr(capBusFactor),
		}, true
	}
	return Gate{}, false
}

func closedToStrangersGate(raw RawMetrics, subs subLookup) (Gate, bool) {
	newcomerSample := raw.NewcomerPRsMerged + raw.NewcomerPRsClosedUnmerged
	if subs.prAcceptance >= 70 && subs.newcomerMergeRate <= 15 && newcomerSample > 0 {
		return Gate{
			Key:      "closed_to_strangers",
			Severity: SeverityWarn,
			Title:    "Closed to newcomers",
			Detail: fmt.Sprintf("Insiders' PRs merge, strangers' don't (%d of %d newcomer PRs). Get a maintainer's agreement in an issue before writing code.",
				raw.NewcomerPRsMerged, newcomerSample),
			HowToClear: "Merge PRs from first-time and non-member contributors.",
			CapTo:      ptr(capStrangers),
		}, true
	}
	return Gate{}, false
}

func staleOrArchivedGate(raw RawMetrics, subs subLookup) (Gate, bool) {
	dead := raw.Archived || raw.Disabled
	stale := raw.DaysSinceLastPush > 365
	if !dead && !stale {
		return Gate{}, false
	}

	if dead {
		return Gate{
			Key:        "stale_or_archived",
			Severity:   SeverityCritical,
			Title:      "Archived or disabled",
			Detail:     "Archived: nothing you send can be merged.",
			HowToClear: "Unarchive the repository to accept changes again.",
			CapTo:      ptr(capArchived),
		}, true
	}

	mature := raw.RepoAgeDays > 365 && subs.issueCloseRatio >= 70 && raw.ReleaseCount > 0
	if mature {
		return Gate{
			Key:        "stale_or_archived",
			Severity:   SeverityInfo,
			Title:      "Mature/stable, low cadence",
			Detail:     "Stable and quiet, not abandoned: expect slow but real reviews.",
			HowToClear: "A small release or a pinned status issue tells contributors the project is alive.",
			CapTo:      nil,
		}, true
	}

	return Gate{
		Key:      "stale_or_archived",
		Severity: SeverityWarn,
		Title:    "Stale",
		Detail: fmt.Sprintf("No commits in %s. Treat as unmaintained unless a maintainer replies to an issue.",
			spanDays(raw.DaysSinceLastPush)),
		HowToClear: "Resume commits or cut a release.",
		CapTo:      ptr(capStale),
	}, true
}

func integrityRiskGate(raw RawMetrics, rawComposite float64) (Gate, bool) {
	if raw.UsesPullRequestTarget && !raw.HasSignedReleaseAssets && rawComposite > 70 {
		return Gate{
			Key:        "integrity_risk",
			Severity:   SeverityWarn,
			Title:      "Supply-chain integrity risk",
			Detail:     "Risky CI trigger plus unsigned releases: your PR could run with repo secrets, and releases can't be verified.",
			HowToClear: "Sign release assets and drop pull_request_target workflows.",
			CapTo:      ptr(capIntegrity),
		}, true
	}
	return Gate{}, false
}

func vanityStarsGate(raw RawMetrics) (Gate, bool) {
	if raw.Stars > 5000 && raw.Watchers*200 < raw.Stars {
		return Gate{
			Key:      "vanity_stars",
			Severity: SeverityInfo,
			Title:    "Stars outpace engagement",
			Detail: fmt.Sprintf("Popular on paper (%s stars, %s watchers). Don't read stars as an active community.",
				humanCount(raw.Stars), humanCount(raw.Watchers)),
			HowToClear: "Grow the watcher and contributor base so engagement matches the star count.",
			CapTo:      nil,
		}, true
	}
	return Gate{}, false
}

func applyCaps(composite float64, gates []Gate) float64 {
	capped := composite
	for _, g := range gates {
		if g.CapTo != nil {
			capped = math.Min(capped, *g.CapTo)
		}
	}
	return capped
}

func ptr(v float64) *float64 { return &v }

func humanCount(n int) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}
