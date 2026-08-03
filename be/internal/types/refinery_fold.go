package types

import "errors"

// RefineryFoldRequest is the shared vocabulary between the refinery package
// and the spawner package for a cli_interactive chain-entry fold attempt:
// refinery.CLIFolder builds the request, spawner.Spawner.RunRefineryFold
// spawns the one-off `_refinery-cli` headless child. Neither package
// imports the other's internals — see refinery/CLAUDE.md's Import Hygiene
// section.
type RefineryFoldRequest struct {
	ProjectID          string
	SessionID          string
	WorkflowInstanceID string
	NodeID             string
	ModelID            string
	Provider           string
	ReasoningEffort    string
	UserText           string
	TimeoutSec         int
}

// RefineryFoldResult is the folded digest content plus token usage and the
// child session id that produced it.
type RefineryFoldResult struct {
	Content        string
	InputTokens    int
	OutputTokens   int
	ChildSessionID string
}

// ErrRefineryFoldProviderBuild sentinel-wraps a build-time provider-construct
// or credential failure in a cli_interactive fold attempt, so refinery's
// chain walk can classify it as advance-eligible exactly like the spawner's
// own build-time provider failure (be/internal/spawner/tier_fallback.go).
var ErrRefineryFoldProviderBuild = errors.New("refinery fold: provider build failure")
