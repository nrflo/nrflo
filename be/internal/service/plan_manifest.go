package service

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
)

// PlanManifest is the planner's output contract (schema version 1). It is
// intentionally minimal: no model, effort, tools, or finding-key fields — those
// are resolved from each node's Template at materialization time (DYNWF-5), so
// the planner cannot smuggle server-controlled configuration through a plan.
type PlanManifest struct {
	Version   int            `json:"version"`
	Goal      string         `json:"goal"`
	Layers    []PlanLayer    `json:"layers"`
	Questions []PlanQuestion `json:"questions,omitempty"`
}

// PlanLayer is one execution layer in the manifest.
type PlanLayer struct {
	Layer  int        `json:"layer"`
	Policy string     `json:"policy"`
	Nodes  []PlanNode `json:"nodes"`
}

// PlanNode is a single node within a layer, bound to a fanout_template agent
// definition by name.
type PlanNode struct {
	ID           string `json:"id"`
	Template     string `json:"template"`
	Instructions string `json:"instructions"`
}

// PlanQuestion is an open question the planner surfaces for the caller to
// answer on a subsequent revise call. Open questions never block approval.
type PlanQuestion struct {
	ID       string `json:"id"`
	Question string `json:"question"`
}

// ParsePlanManifest decodes raw JSON into a PlanManifest, rejecting any field
// not in the v1 schema (DisallowUnknownFields) before semantic validation runs.
func ParsePlanManifest(raw json.RawMessage) (PlanManifest, error) {
	var m PlanManifest
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&m); err != nil {
		return PlanManifest{}, fmt.Errorf("manifest is not valid JSON for schema version 1: %w", err)
	}
	return m, nil
}

// HashManifest returns the sha256 hex digest of the manifest's canonical
// (re-marshalled, deterministic field order) JSON representation.
func HashManifest(m PlanManifest) string {
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
