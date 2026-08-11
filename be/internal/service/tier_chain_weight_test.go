package service

import (
	"reflect"
	"testing"
)

func weightedChain() []AgentChainEntry {
	return []AgentChainEntry{
		{ModelID: "a", Position: 0, Weight: 50},
		{ModelID: "b", Position: 1, Weight: 30},
		{ModelID: "c", Position: 2, Weight: 20},
	}
}

func orderIDs(chain []AgentChainEntry) []string {
	ids := make([]string, len(chain))
	for i, e := range chain {
		ids[i] = e.ModelID
	}
	return ids
}

func TestWeightedChainOrder_NoWeightsUnchanged(t *testing.T) {
	chain := []AgentChainEntry{{ModelID: "a", Position: 0}, {ModelID: "b", Position: 1}}
	got := WeightedChainOrder(chain, map[int]int{0: 100}, nil)
	if !reflect.DeepEqual(orderIDs(got), []string{"a", "b"}) {
		t.Errorf("order = %v, want unchanged [a b]", orderIDs(got))
	}
}

func TestWeightedChainOrder_NoCountsPicksHighestWeight(t *testing.T) {
	chain := []AgentChainEntry{
		{ModelID: "a", Position: 0, Weight: 10},
		{ModelID: "b", Position: 1, Weight: 90},
	}
	got := WeightedChainOrder(chain, map[int]int{}, nil)
	if !reflect.DeepEqual(orderIDs(got), []string{"b", "a"}) {
		t.Errorf("order = %v, want [b a] (highest weight leads with no history)", orderIDs(got))
	}
}

func TestWeightedChainOrder_DeficitPick(t *testing.T) {
	// a is over-served (5 of 7 landings vs a 50% target); b has the largest
	// deficit (30% target, 1/7 actual) and must lead; the rest stay ordinal.
	got := WeightedChainOrder(weightedChain(), map[int]int{0: 5, 1: 1, 2: 1}, nil)
	if !reflect.DeepEqual(orderIDs(got), []string{"b", "a", "c"}) {
		t.Errorf("order = %v, want [b a c]", orderIDs(got))
	}
}

func TestWeightedChainOrder_TieResolvesToLowestPosition(t *testing.T) {
	chain := []AgentChainEntry{
		{ModelID: "a", Position: 0, Weight: 50},
		{ModelID: "b", Position: 1, Weight: 50},
	}
	got := WeightedChainOrder(chain, map[int]int{}, nil)
	if !reflect.DeepEqual(orderIDs(got), []string{"a", "b"}) {
		t.Errorf("order = %v, want [a b] (deterministic tie-break)", orderIDs(got))
	}
}

func TestWeightedChainOrder_IneligibleEntryNeverStarts(t *testing.T) {
	chain := []AgentChainEntry{
		{ModelID: "a", Position: 0, Weight: 10},
		{ModelID: "b", Position: 1, Weight: 90},
	}
	got := WeightedChainOrder(chain, map[int]int{}, func(e AgentChainEntry) bool { return e.ModelID != "b" })
	if !reflect.DeepEqual(orderIDs(got), []string{"a", "b"}) {
		t.Errorf("order = %v, want [a b] (ineligible b excluded from the pick)", orderIDs(got))
	}
}

func TestWeightedChainOrder_AllWeightedIneligibleUnchanged(t *testing.T) {
	got := WeightedChainOrder(weightedChain(), map[int]int{}, func(AgentChainEntry) bool { return false })
	if !reflect.DeepEqual(orderIDs(got), []string{"a", "b", "c"}) {
		t.Errorf("order = %v, want unchanged ordinal", orderIDs(got))
	}
}

func TestWeightedChainOrder_WeightlessEntriesStayFallbackOnly(t *testing.T) {
	// Only c is weighted: it leads, a/b remain the ordinal fallback path.
	chain := []AgentChainEntry{
		{ModelID: "a", Position: 0},
		{ModelID: "b", Position: 1},
		{ModelID: "c", Position: 2, Weight: 100},
	}
	got := WeightedChainOrder(chain, map[int]int{}, nil)
	if !reflect.DeepEqual(orderIDs(got), []string{"c", "a", "b"}) {
		t.Errorf("order = %v, want [c a b]", orderIDs(got))
	}
}
