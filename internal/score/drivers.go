package score

import "sort"

// driversN is how many strongest and weakest sub-scores Drivers returns.
const driversN = 3

// Drivers returns the strongest and weakest sub-scores across all categories,
// up to driversN each, ordered best-first and worst-first respectively. It is
// pure and deterministic: ties keep category/sub-score order (stable sort).
//
// ceiling: with 15 sub-scores and driversN=3, value-distinct sets never
// overlap; all-equal ties can produce identical strong and weak sets. If
// driversN ever exceeds half the sub-score count, also dedupe distinct-value
// overlap.
func Drivers(r Report) (strong, weak []SubScore) {
	var all []SubScore
	for _, c := range r.Categories {
		all = append(all, c.Subs...)
	}
	if len(all) == 0 {
		return nil, nil
	}

	byDesc := make([]SubScore, len(all))
	copy(byDesc, all)
	sort.SliceStable(byDesc, func(i, j int) bool { return byDesc[i].Value > byDesc[j].Value })

	byAsc := make([]SubScore, len(all))
	copy(byAsc, all)
	sort.SliceStable(byAsc, func(i, j int) bool { return byAsc[i].Value < byAsc[j].Value })

	n := driversN
	if n > len(all) {
		n = len(all)
	}
	return byDesc[:n], byAsc[:n]
}
