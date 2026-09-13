// Package output renders a scored report for non-interactive use.
package output

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/jameszmapepa/worthy/internal/score"
)

// Document is the JSON shape written by JSON.
type Document struct {
	Repository string     `json:"repository"`
	URL        string     `json:"url"`
	Grade      string     `json:"grade"`
	Score      float64    `json:"score"`
	Composite  float64    `json:"composite_before_gates"`
	Verdict    string     `json:"verdict"`
	Confidence string     `json:"confidence"`
	Questions  []Question `json:"questions"`
	Categories []Category `json:"categories"`
	Gates      []Gate     `json:"gates"`
	Partial    []string   `json:"partial_metrics"`
	Repo       RepoMeta   `json:"repo"`
}

// Question is one of the two headline questions.
type Question struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Grade   string  `json:"grade"`
	Score   float64 `json:"score"`
	Message string  `json:"message,omitempty"`
}

// Category is a weighted group of indicators.
type Category struct {
	Key        string      `json:"key"`
	Label      string      `json:"label"`
	Score      float64     `json:"score"`
	Weight     float64     `json:"weight"`
	Indicators []Indicator `json:"indicators"`
}

// Indicator is a single scored signal with its raw reading and formula.
type Indicator struct {
	Key     string   `json:"key"`
	Label   string   `json:"label"`
	Score   float64  `json:"score"`
	Grade   string   `json:"grade"`
	Raw     string   `json:"raw"`
	Formula string   `json:"formula"`
	Weight  float64  `json:"weight"`
	Gates   []string `json:"gates,omitempty"`
}

// Gate is an anti-gaming rule that fired.
type Gate struct {
	Key        string   `json:"key"`
	Severity   string   `json:"severity"`
	Title      string   `json:"title"`
	Detail     string   `json:"detail"`
	HowToClear string   `json:"how_to_clear,omitempty"`
	CapsTo     *float64 `json:"caps_score_to,omitempty"`
}

// RepoMeta is the header metadata.
type RepoMeta struct {
	Description string `json:"description"`
	Language    string `json:"language"`
	License     string `json:"license"`
	Stars       int    `json:"stars"`
	Forks       int    `json:"forks"`
	Watchers    int    `json:"watchers"`
	AgeDays     int    `json:"age_days"`
	Archived    bool   `json:"archived"`
}

// Build converts a report into the JSON document.
func Build(owner, repo string, r score.Report, raw score.RawMetrics) Document {
	d := Document{
		Repository: owner + "/" + repo,
		URL:        "https://github.com/" + owner + "/" + repo,
		Grade:      r.Grade,
		Score:      r.AdjustedComposite,
		Composite:  r.Composite,
		Verdict:    r.Verdict,
		Confidence: strings.ToLower(r.Confidence.String()),
		Questions:  make([]Question, 0, 2),
		Categories: make([]Category, 0, len(r.Categories)),
		Gates:      make([]Gate, 0, len(r.Gates)),
		Partial:    append([]string{}, raw.Partial...),
		Repo: RepoMeta{
			Description: raw.Description, Language: raw.Language, License: raw.LicenseSPDX,
			Stars: raw.Stars, Forks: raw.Forks, Watchers: raw.Watchers, AgeDays: raw.RepoAgeDays, Archived: raw.Archived,
		},
	}
	for _, q := range []score.QuestionScore{r.Maintained, r.Contributable} {
		d.Questions = append(d.Questions, Question{Key: q.Key, Label: q.Label, Grade: q.Grade, Score: q.Value, Message: q.Message})
	}
	for _, c := range r.Categories {
		cat := Category{Key: c.Key, Label: c.Label, Score: c.Value, Weight: c.Weight, Indicators: make([]Indicator, 0, len(c.Subs))}
		for _, s := range c.Subs {
			cat.Indicators = append(cat.Indicators, Indicator{
				Key: s.Key, Label: s.Label, Score: s.Value, Grade: score.LetterGrade(s.Value),
				Raw: s.Raw, Formula: s.Formula, Weight: s.Weight, Gates: s.Gates,
			})
		}
		d.Categories = append(d.Categories, cat)
	}
	for _, g := range r.Gates {
		d.Gates = append(d.Gates, Gate{Key: g.Key, Severity: g.Severity, Title: g.Title, Detail: g.Detail, HowToClear: g.HowToClear, CapsTo: g.CapTo})
	}
	return d
}

// JSON writes the indented document followed by a newline.
func JSON(w io.Writer, owner, repo string, r score.Report, raw score.RawMetrics) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(Build(owner, repo, r, raw))
}
