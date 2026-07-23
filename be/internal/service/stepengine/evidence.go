package stepengine

import (
	"fmt"
	"strings"

	"be/internal/handoff"
	"be/internal/model"
)

// EvidenceContext identifies the run + working tree ValidateEvidence checks
// findings and path-bearing values against.
type EvidenceContext struct {
	InstanceID string
	NodeID     string
	SessionID  string
	RepoRoot   string
}

// KeyProblem names a required_findings key whose value failed its schema.
type KeyProblem struct {
	Key     string
	Schema  string
	Problem string
}

// EvidenceResult is the outcome of validating one step's required_findings
// against the node's current findings.
type EvidenceResult struct {
	OK      bool
	Missing []string
	Invalid []KeyProblem
	// Overlaps names paths claimed by both sides of a step's path_overlap
	// gate (e.g. a file in both be_files_to_modify and fe_files_to_modify) —
	// unlike Flags, a non-empty Overlaps blocks completion.
	Overlaps []string
	// Flags carries non-fatal path-resolution notices (ambiguous or
	// unresolved path-bearing values) — these never block completion, since
	// e.g. files_to_create legitimately does not exist yet.
	Flags []string
}

// ValidateEvidence checks every step.RequiredFindings key is present on the
// node and validates its value against the key's named schema. An unknown
// node (no session ever attributed to it) is a Go error; missing/invalid
// keys are reported in the returned EvidenceResult, never as an error.
func (e *Engine) ValidateEvidence(step model.StepDefinition, ec EvidenceContext) (EvidenceResult, error) {
	findings, exists, err := e.findingRepo.GetByNode(ec.InstanceID, ec.NodeID)
	if err != nil {
		return EvidenceResult{}, fmt.Errorf("stepengine: load findings for node %s: %w", ec.NodeID, err)
	}
	if !exists {
		return EvidenceResult{}, fmt.Errorf("stepengine: unknown node %q", ec.NodeID)
	}

	var result EvidenceResult
	var allPaths []string
	for _, rf := range step.RequiredFindings {
		raw, ok := findings[rf.Key]
		if !ok {
			result.Missing = append(result.Missing, rf.Key)
			continue
		}
		paths, err := validateSchemaValue(rf.Schema, raw)
		if err != nil {
			result.Invalid = append(result.Invalid, KeyProblem{Key: rf.Key, Schema: rf.Schema, Problem: err.Error()})
			continue
		}
		allPaths = append(allPaths, paths...)
	}

	for _, pr := range handoff.ResolvePathCandidates(ec.RepoRoot, allPaths) {
		if pr.Status != handoff.PathResolved {
			result.Flags = append(result.Flags, fmt.Sprintf("path %q could not be uniquely resolved under the worktree (non-fatal)", pr.Candidate))
		}
	}

	result.Overlaps = checkPathOverlap(findings, step.PathOverlap)

	result.OK = len(result.Missing) == 0 && len(result.Invalid) == 0 && len(result.Overlaps) == 0
	return result, nil
}

// RejectionMessage builds one aggregated agent-facing message enumerating
// every missing key and every schema failure together — a rejection must
// never dribble out one problem per retry.
func (r EvidenceResult) RejectionMessage() string {
	var b strings.Builder
	if len(r.Missing) > 0 {
		b.WriteString("missing required findings: " + strings.Join(r.Missing, ", "))
	}
	for _, p := range r.Invalid {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "finding %q failed schema %q: %s", p.Key, p.Schema, p.Problem)
	}
	if len(r.Overlaps) > 0 {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		fmt.Fprintf(&b, "files claimed by both sides: %s — assign each file to exactly one side", strings.Join(r.Overlaps, ", "))
	}
	return b.String()
}

// RejectionReason names the single reason a non-OK EvidenceResult should be
// rejected under, in fixed priority order (Rule 6: the reason-selection
// switch lives on the type, not scattered at call sites): missing evidence
// first, then invalid schema, then a path_overlap gate failure.
func (r EvidenceResult) RejectionReason() string {
	switch {
	case len(r.Missing) > 0:
		return "missing_evidence"
	case len(r.Invalid) > 0:
		return "invalid_evidence"
	case len(r.Overlaps) > 0:
		return "path_overlap"
	default:
		return ""
	}
}
