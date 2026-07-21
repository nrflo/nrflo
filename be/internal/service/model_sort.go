package service

import (
	"sort"

	"be/internal/model"
)

// SortModelsForPicker orders models newest-release-first for the console
// catalog: ISO ReleaseDate strings compare chronologically, so a plain
// descending string sort works; ""/unknown sorts last. Ties (including all
// rows with no release_date) break on capability via the single
// PlanModelTierClass classifier (Premium > Mid > Cheap) — no call-site
// name-check (Rule 6). Stable so equal-rank rows keep their input order.
func SortModelsForPicker(models []*model.Model) {
	sort.SliceStable(models, func(i, j int) bool {
		return CompareModelsForPicker(models[i], models[j]) < 0
	})
}

// CompareModelsForPicker returns <0 if a sorts before b, >0 if after, 0 if
// equal-rank. See SortModelsForPicker for the ordering rules.
func CompareModelsForPicker(a, b *model.Model) int {
	switch {
	case a.ReleaseDate == "" && b.ReleaseDate == "":
		// fall through to tier tie-break
	case a.ReleaseDate == "":
		return 1
	case b.ReleaseDate == "":
		return -1
	case a.ReleaseDate != b.ReleaseDate:
		if a.ReleaseDate > b.ReleaseDate {
			return -1
		}
		return 1
	}

	tierA, tierB := PlanModelTierClass(a), PlanModelTierClass(b)
	if tierA != tierB {
		if tierA > tierB {
			return -1
		}
		return 1
	}
	return 0
}
