package service

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// LayerPolicy is a parsed representation of a pass_policy string.
type LayerPolicy struct {
	Kind    string // "any", "all", "quorum", "percent"
	N       int    // quorum count (Kind=="quorum")
	Percent int    // percent value 1-100 (Kind=="percent")
}

// ParseLayerPolicy parses a raw policy string into a LayerPolicy.
// Empty string and "any" are both valid and map to Kind="any".
func ParseLayerPolicy(s string) (LayerPolicy, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "any" {
		return LayerPolicy{Kind: "any"}, nil
	}
	if s == "all" {
		return LayerPolicy{Kind: "all"}, nil
	}
	if strings.HasPrefix(s, "quorum:") {
		payload := strings.TrimPrefix(s, "quorum:")
		n, err := strconv.Atoi(payload)
		if err != nil {
			return LayerPolicy{}, fmt.Errorf("invalid quorum payload %q: must be an integer", payload)
		}
		return LayerPolicy{Kind: "quorum", N: n}, nil
	}
	if strings.HasPrefix(s, "percent:") {
		payload := strings.TrimPrefix(s, "percent:")
		p, err := strconv.Atoi(payload)
		if err != nil {
			return LayerPolicy{}, fmt.Errorf("invalid percent payload %q: must be an integer", payload)
		}
		return LayerPolicy{Kind: "percent", Percent: p}, nil
	}
	return LayerPolicy{}, fmt.Errorf("unknown pass_policy %q", s)
}

// Required returns the number of agents that must pass given denom total agents.
func (lp LayerPolicy) Required(denom int) int {
	switch lp.Kind {
	case "any":
		return 1
	case "all":
		return denom
	case "quorum":
		return lp.N
	case "percent":
		return int(math.Ceil(float64(denom) * float64(lp.Percent) / 100.0))
	default:
		return 1
	}
}

// String returns the canonical string form of the policy.
func (lp LayerPolicy) String() string {
	switch lp.Kind {
	case "all":
		return "all"
	case "quorum":
		return fmt.Sprintf("quorum:%d", lp.N)
	case "percent":
		return fmt.Sprintf("percent:%d", lp.Percent)
	default:
		return "any"
	}
}

// ValidateLayerPolicy validates a raw policy string against the number of agents in the layer.
func ValidateLayerPolicy(s string, layerAgentCount int) error {
	lp, err := ParseLayerPolicy(s)
	if err != nil {
		return err
	}
	switch lp.Kind {
	case "quorum":
		if lp.N <= 0 {
			return fmt.Errorf("quorum must be >= 1")
		}
		if lp.N > layerAgentCount {
			return fmt.Errorf("quorum %d exceeds agent count %d in this layer", lp.N, layerAgentCount)
		}
	case "percent":
		if lp.Percent < 1 {
			return fmt.Errorf("percent must be >= 1")
		}
		if lp.Percent > 100 {
			return fmt.Errorf("percent must be <= 100")
		}
	}
	return nil
}
