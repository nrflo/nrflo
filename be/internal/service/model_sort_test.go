package service

import (
	"testing"

	"be/internal/model"
)

func floatPtr(f float64) *float64 { return &f }

// TestSortModelsForPicker_NewestReleaseFirst verifies rows with distinct
// release dates sort strictly newest-first, regardless of input order.
func TestSortModelsForPicker_NewestReleaseFirst(t *testing.T) {
	oldest := &model.Model{ID: "old", ReleaseDate: "2026-01-01"}
	mid := &model.Model{ID: "mid", ReleaseDate: "2026-06-01"}
	newest := &model.Model{ID: "new", ReleaseDate: "2026-07-20"}

	models := []*model.Model{mid, oldest, newest}
	SortModelsForPicker(models)

	if models[0].ID != "new" || models[1].ID != "mid" || models[2].ID != "old" {
		t.Fatalf("order = [%s %s %s], want [new mid old]", models[0].ID, models[1].ID, models[2].ID)
	}
}

// TestSortModelsForPicker_EqualDateTieBreaksOnTier verifies same-release-date
// rows break ties on PlanModelTierClass (Premium > Mid > Cheap), covering
// both a priced pair (PriceIn-driven) and an unpriced name-class pair.
func TestSortModelsForPicker_EqualDateTieBreaksOnTier(t *testing.T) {
	t.Run("priced pair", func(t *testing.T) {
		cheap := &model.Model{ID: "haiku-priced", ReleaseDate: "2026-07-01", PriceIn: floatPtr(1)}
		premium := &model.Model{ID: "opus-priced", ReleaseDate: "2026-07-01", PriceIn: floatPtr(10)}

		models := []*model.Model{cheap, premium}
		SortModelsForPicker(models)

		if models[0].ID != "opus-priced" || models[1].ID != "haiku-priced" {
			t.Fatalf("order = [%s %s], want [opus-priced haiku-priced]", models[0].ID, models[1].ID)
		}
	})

	t.Run("unpriced name-class pair", func(t *testing.T) {
		haiku := &model.Model{ID: "some-haiku-model", ReleaseDate: "2026-07-01"}
		gpt := &model.Model{ID: "some-gpt-model", ReleaseDate: "2026-07-01"}
		opus := &model.Model{ID: "some-opus-model", ReleaseDate: "2026-07-01"}

		models := []*model.Model{haiku, gpt, opus}
		SortModelsForPicker(models)

		if models[0].ID != "some-opus-model" {
			t.Fatalf("models[0] = %s, want some-opus-model (premium tier first)", models[0].ID)
		}
		if models[1].ID != "some-gpt-model" {
			t.Fatalf("models[1] = %s, want some-gpt-model (mid tier second)", models[1].ID)
		}
		if models[2].ID != "some-haiku-model" {
			t.Fatalf("models[2] = %s, want some-haiku-model (cheap tier last)", models[2].ID)
		}
	})
}

// TestSortModelsForPicker_UnknownDateSortsLastAndStable verifies rows with
// empty ReleaseDate sort after every dated row, and that equal-rank rows
// (same date+tier, or all-empty-date+same-tier) preserve their relative
// input order (sort.SliceStable).
func TestSortModelsForPicker_UnknownDateSortsLastAndStable(t *testing.T) {
	dated := &model.Model{ID: "dated", ReleaseDate: "2026-01-01"}
	unknownA := &model.Model{ID: "unknown-a", ReleaseDate: ""}
	unknownB := &model.Model{ID: "unknown-b", ReleaseDate: ""}
	unknownC := &model.Model{ID: "unknown-c", ReleaseDate: ""}

	models := []*model.Model{unknownA, unknownB, dated, unknownC}
	SortModelsForPicker(models)

	if models[0].ID != "dated" {
		t.Fatalf("models[0] = %s, want dated (only row with a known release_date)", models[0].ID)
	}
	wantTail := []string{"unknown-a", "unknown-b", "unknown-c"}
	for i, want := range wantTail {
		if got := models[i+1].ID; got != want {
			t.Errorf("models[%d] = %s, want %s (input order preserved among unknown-date rows)", i+1, got, want)
		}
	}
}

// TestCompareModelsForPicker_TableDriven exercises CompareModelsForPicker
// directly for the sign conventions SortModelsForPicker relies on.
func TestCompareModelsForPicker_TableDriven(t *testing.T) {
	tests := []struct {
		name    string
		a, b    *model.Model
		wantSgn int // -1, 0, 1 (sign of result)
	}{
		{
			name:    "newer release_date sorts before older",
			a:       &model.Model{ID: "a", ReleaseDate: "2026-07-20"},
			b:       &model.Model{ID: "b", ReleaseDate: "2026-01-01"},
			wantSgn: -1,
		},
		{
			name:    "older release_date sorts after newer",
			a:       &model.Model{ID: "a", ReleaseDate: "2026-01-01"},
			b:       &model.Model{ID: "b", ReleaseDate: "2026-07-20"},
			wantSgn: 1,
		},
		{
			name:    "known date beats unknown date",
			a:       &model.Model{ID: "a", ReleaseDate: "2026-01-01"},
			b:       &model.Model{ID: "b", ReleaseDate: ""},
			wantSgn: -1,
		},
		{
			name:    "unknown date loses to known date",
			a:       &model.Model{ID: "a", ReleaseDate: ""},
			b:       &model.Model{ID: "b", ReleaseDate: "2026-01-01"},
			wantSgn: 1,
		},
		{
			name:    "equal date and tier is a tie",
			a:       &model.Model{ID: "a", ReleaseDate: "2026-01-01", PriceIn: floatPtr(1)},
			b:       &model.Model{ID: "b", ReleaseDate: "2026-01-01", PriceIn: floatPtr(1)},
			wantSgn: 0,
		},
		{
			name:    "both unknown date, equal tier is a tie",
			a:       &model.Model{ID: "some-gpt-a"},
			b:       &model.Model{ID: "some-gpt-b"},
			wantSgn: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CompareModelsForPicker(tt.a, tt.b)
			gotSgn := 0
			switch {
			case got < 0:
				gotSgn = -1
			case got > 0:
				gotSgn = 1
			}
			if gotSgn != tt.wantSgn {
				t.Errorf("CompareModelsForPicker(%s, %s) = %d (sign %d), want sign %d", tt.a.ID, tt.b.ID, got, gotSgn, tt.wantSgn)
			}
		})
	}
}
