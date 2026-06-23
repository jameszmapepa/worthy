package score

import "sort"

const driversN = 3

// Drivers returns the top and bottom driversN sub-scores across all categories, ordered best-first and worst-first; ties preserve category/sub-score order.
// ceiling: if driversN ever exceeds half the sub-score count, dedupe value overlap between strong and weak sets.
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

	n := min(driversN, len(all))
	return byDesc[:n], byAsc[:n]
}
