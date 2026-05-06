package service

import (
	"math"
	"testing"
)

func TestParseLayerPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input    string
		wantKind string
		wantN    int
		wantPct  int
		wantErr  bool
	}{
		{"", "any", 0, 0, false},
		{"any", "any", 0, 0, false},
		{"all", "all", 0, 0, false},
		{"quorum:2", "quorum", 2, 0, false},
		{"quorum:1", "quorum", 1, 0, false},
		{"percent:80", "percent", 0, 80, false},
		{"percent:1", "percent", 0, 1, false},
		{"percent:100", "percent", 0, 100, false},
		{"quorum:abc", "", 0, 0, true},
		{"foo", "", 0, 0, true},
		{"quorum", "", 0, 0, true},
		{"percent:", "", 0, 0, true},
		{"percent:abc", "", 0, 0, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input+"_"+tc.wantKind, func(t *testing.T) {
			t.Parallel()
			lp, err := ParseLayerPolicy(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Errorf("ParseLayerPolicy(%q) = nil error, want error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseLayerPolicy(%q) unexpected error: %v", tc.input, err)
			}
			if lp.Kind != tc.wantKind {
				t.Errorf("Kind = %q, want %q", lp.Kind, tc.wantKind)
			}
			if tc.wantKind == "quorum" && lp.N != tc.wantN {
				t.Errorf("N = %d, want %d", lp.N, tc.wantN)
			}
			if tc.wantKind == "percent" && lp.Percent != tc.wantPct {
				t.Errorf("Percent = %d, want %d", lp.Percent, tc.wantPct)
			}
		})
	}
}

func TestLayerPolicy_Required_Any(t *testing.T) {
	t.Parallel()
	lp := LayerPolicy{Kind: "any"}
	cases := []struct{ denom, want int }{{0, 1}, {5, 1}, {100, 1}}
	for _, tc := range cases {
		if got := lp.Required(tc.denom); got != tc.want {
			t.Errorf("any.Required(%d) = %d, want %d", tc.denom, got, tc.want)
		}
	}
}

func TestLayerPolicy_Required_All(t *testing.T) {
	t.Parallel()
	lp := LayerPolicy{Kind: "all"}
	cases := []struct{ denom, want int }{{0, 0}, {1, 1}, {3, 3}}
	for _, tc := range cases {
		if got := lp.Required(tc.denom); got != tc.want {
			t.Errorf("all.Required(%d) = %d, want %d", tc.denom, got, tc.want)
		}
	}
}

func TestLayerPolicy_Required_Quorum(t *testing.T) {
	t.Parallel()
	lp := LayerPolicy{Kind: "quorum", N: 2}
	cases := []struct{ denom, want int }{{2, 2}, {5, 2}, {10, 2}}
	for _, tc := range cases {
		if got := lp.Required(tc.denom); got != tc.want {
			t.Errorf("quorum:2.Required(%d) = %d, want %d", tc.denom, got, tc.want)
		}
	}
}

func TestLayerPolicy_Required_Percent(t *testing.T) {
	t.Parallel()
	lp := LayerPolicy{Kind: "percent", Percent: 80}
	cases := []struct {
		denom int
		want  int
	}{
		{3, int(math.Ceil(float64(3) * 80.0 / 100.0))}, // ceil(2.4) = 3
		{5, int(math.Ceil(float64(5) * 80.0 / 100.0))}, // ceil(4.0) = 4
	}
	for _, tc := range cases {
		if got := lp.Required(tc.denom); got != tc.want {
			t.Errorf("percent:80.Required(%d) = %d, want %d", tc.denom, got, tc.want)
		}
	}
}

func TestValidateLayerPolicy(t *testing.T) {
	t.Parallel()
	cases := []struct {
		policy  string
		count   int
		wantErr bool
	}{
		{"quorum:2", 2, false},
		{"quorum:1", 3, false},
		{"quorum:3", 2, true},
		{"quorum:0", 2, true},
		{"percent:1", 5, false},
		{"percent:100", 5, false},
		{"percent:0", 5, true},
		{"percent:101", 5, true},
		{"percent:-5", 5, true},
		{"any", 0, false},
		{"", 0, false},
		{"all", 0, false},
		{"foo", 2, true},
		{"quorum:abc", 2, true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.policy, func(t *testing.T) {
			t.Parallel()
			err := ValidateLayerPolicy(tc.policy, tc.count)
			if (err != nil) != tc.wantErr {
				t.Errorf("ValidateLayerPolicy(%q, %d) error = %v, wantErr = %v",
					tc.policy, tc.count, err, tc.wantErr)
			}
		})
	}
}

func TestLayerPolicy_String(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"", "any"},
		{"any", "any"},
		{"all", "all"},
		{"quorum:3", "quorum:3"},
		{"percent:80", "percent:80"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input+"_"+tc.want, func(t *testing.T) {
			t.Parallel()
			lp, err := ParseLayerPolicy(tc.input)
			if err != nil {
				t.Fatalf("ParseLayerPolicy(%q) error: %v", tc.input, err)
			}
			if got := lp.String(); got != tc.want {
				t.Errorf("String() = %q, want %q", got, tc.want)
			}
		})
	}
}
