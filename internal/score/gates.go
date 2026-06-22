package score

import "math"

// Gate severities.
const (
	SeverityInfo     = "info"
	SeverityWarn     = "warn"
	SeverityCritical = "critical"
)

// Gate caps. Pointers are used so a "no cap" gate (info) can be distinguished
// from a cap of 0.
var (
	capBusFactor = 70.0
	capStrangers = 75.0
	capArchived  = 40.0
	capStale     = 60.0
	capIntegrity = 80.0
)

// Gate is a conditional annotation on a Report. A non-nil CapTo additionally
// caps the adjusted composite at that value.
type Gate struct {
	Key        string   // stable identifier
	Severity   string   // info | warn | critical
	Title      string   // short headline
	Detail     string   // one-line plain-language explanation
	HowToClear string   // one-line advisory on what would clear the gate
	CapTo      *float64 // composite ceiling imposed by this gate, or nil for none
}

// subLookup carries the few sub-score values that gate predicates need, so
// gates do not recompute them.
type subLookup struct {
	issueCloseRatio   float64
	prAcceptance      float64
	newcomerMergeRate float64
}

// evaluateGates returns the gates triggered by raw given the already-computed
// raw composite and the sub-scores gates depend on. The slice is deterministic
// and ordered.
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

// busFactorGate fires when a single contributor dominates recent commits and
// the contributor pool is tiny.
func busFactorGate(raw RawMetrics) (Gate, bool) {
	if raw.TopContributorRecentShare > 0.80 && raw.ContributorCount <= 2 {
		return Gate{
			Key:        "bus_factor",
			Severity:   SeverityWarn,
			Title:      "Bus factor risk",
			Detail:     "One contributor authors most recent commits with few others involved.",
			HowToClear: "Distribute commits beyond the top author and grow the contributor base.",
			CapTo:      ptr(capBusFactor),
		}, true
	}
	return Gate{}, false
}

// closedToStrangersGate fires when the project merges insiders' PRs readily but
// rarely merges newcomers'.
func closedToStrangersGate(raw RawMetrics, subs subLookup) (Gate, bool) {
	newcomerSample := raw.NewcomerPRsMerged + raw.NewcomerPRsClosedUnmerged
	if subs.prAcceptance >= 70 && subs.newcomerMergeRate <= 15 && newcomerSample > 0 {
		return Gate{
			Key:        "closed_to_strangers",
			Severity:   SeverityWarn,
			Title:      "Closed to newcomers",
			Detail:     "PRs are accepted overall but newcomers' PRs are rarely merged.",
			HowToClear: "Merge PRs from first-time and non-member contributors.",
			CapTo:      ptr(capStrangers),
		}, true
	}
	return Gate{}, false
}

// staleOrArchivedGate fires on archived, disabled, or long-unpushed repos. A
// mature, stable, low-cadence repo is downgraded to an informational note with
// no cap (the phase reclassification).
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
			Detail:     "The repository is archived or disabled and no longer accepts changes.",
			HowToClear: "Archived in place; informational only.",
			CapTo:      ptr(capArchived),
		}, true
	}

	// stale (not dead): apply the phase reclassification when the repo is a
	// mature, stable project that has simply slowed down.
	mature := raw.RepoAgeDays > 365 && subs.issueCloseRatio >= 70 && raw.ReleaseCount > 0
	if mature {
		return Gate{
			Key:        "stale_or_archived",
			Severity:   SeverityInfo,
			Title:      "Mature/stable, low cadence",
			Detail:     "An established project with few recent pushes; likely stable rather than abandoned.",
			HowToClear: "Informational: established project, low recent activity.",
			CapTo:      nil,
		}, true
	}

	return Gate{
		Key:        "stale_or_archived",
		Severity:   SeverityWarn,
		Title:      "Stale",
		Detail:     "No pushes in over a year; the project may be unmaintained.",
		HowToClear: "Resume commits or cut a release.",
		CapTo:      ptr(capStale),
	}, true
}

// integrityRiskGate fires when a workflow uses pull_request_target without
// signed release assets on an otherwise high-scoring repo.
func integrityRiskGate(raw RawMetrics, rawComposite float64) (Gate, bool) {
	if raw.UsesPullRequestTarget && !raw.HasSignedReleaseAssets && rawComposite > 70 {
		return Gate{
			Key:        "integrity_risk",
			Severity:   SeverityWarn,
			Title:      "Supply-chain integrity risk",
			Detail:     "Uses pull_request_target and ships unsigned release assets.",
			HowToClear: "Sign release assets and drop pull_request_target workflows.",
			CapTo:      ptr(capIntegrity),
		}, true
	}
	return Gate{}, false
}

// vanityStarsGate flags repos whose star count far outstrips their watchers.
func vanityStarsGate(raw RawMetrics) (Gate, bool) {
	if raw.Stars > 5000 && raw.Watchers*200 < raw.Stars {
		return Gate{
			Key:        "vanity_stars",
			Severity:   SeverityInfo,
			Title:      "Stars outpace engagement",
			Detail:     "High star count relative to watchers; popularity may exceed active use.",
			HowToClear: "Informational: stars are high relative to watchers.",
			CapTo:      nil,
		}, true
	}
	return Gate{}, false
}

// applyCaps returns the composite capped at the minimum of all gate CapTo
// values, never exceeding the raw composite.
func applyCaps(composite float64, gates []Gate) float64 {
	capped := composite
	for _, g := range gates {
		if g.CapTo != nil {
			capped = math.Min(capped, *g.CapTo)
		}
	}
	return capped
}

// ptr returns a pointer to a copy of v.
func ptr(v float64) *float64 { return &v }
