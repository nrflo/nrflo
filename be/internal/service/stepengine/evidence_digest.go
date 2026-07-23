package stepengine

import (
	"bytes"
	"encoding/json"

	"be/internal/handoff"
	"be/internal/model"
)

// EvidenceValue is one required_findings key's recorded value for a
// completed step, rendered as a display string (not raw prompt prose — that
// stays in the spawner).
type EvidenceValue struct {
	Key    string
	Schema string
	Value  string
}

// StepEvidence is the structured, per-completed-step evidence record for one
// stepwise cursor: the snapshot-declared required_findings (authoritative —
// never the agent-supplied CompletedStep.EvidenceKeys) plus their stored
// values and the resolved/unresolved split of any path-bearing values.
type StepEvidence struct {
	Index           int
	StepID          string
	Title           string
	Summary         string
	Findings        []EvidenceValue
	ResolvedPaths   []string
	UnresolvedPaths []string
}

// CompletedEvidence returns one StepEvidence entry per completed step in
// cursor order, built entirely from the immutable steps_snapshot and the
// node's current findings — never the live agent_definitions.steps, so a
// mid-run def edit cannot change what a relaunch prompt sees. Returns
// ErrNoCursor when no cursor exists for (instanceID, nodeID), an empty slice
// (no error) when nothing has completed yet.
func (e *Engine) CompletedEvidence(instanceID, nodeID string) ([]StepEvidence, error) {
	cursor, err := e.cursorRepo.Get(instanceID, nodeID)
	if err != nil {
		return nil, ErrNoCursor
	}
	steps, err := decodeSteps([]byte(cursor.StepsSnapshot))
	if err != nil {
		return nil, ErrBadSnapshot
	}
	completed, err := decodeCompleted(cursor.Completed)
	if err != nil {
		return nil, ErrBadSnapshot
	}
	if len(completed) == 0 {
		return nil, nil
	}

	stepByID := make(map[string]int, len(steps))
	for i, st := range steps {
		stepByID[st.StepID] = i
	}

	findings, _, err := e.findingRepo.GetByNode(instanceID, nodeID)
	if err != nil {
		findings = nil
	}
	root := e.resolveWorktreeRoot(instanceID)

	result := make([]StepEvidence, 0, len(completed))
	for _, cs := range completed {
		idx, ok := stepByID[cs.StepID]
		if !ok {
			continue
		}
		step := steps[idx]

		var allPaths []string
		ev := StepEvidence{Index: idx, StepID: step.StepID, Title: step.Title, Summary: cs.Summary}
		for _, rf := range step.RequiredFindings {
			raw, present := findings[rf.Key]
			value := ""
			if present {
				value = renderFindingValue(rf.Schema, raw)
				if paths, err := validateSchemaValue(rf.Schema, raw); err == nil {
					allPaths = append(allPaths, paths...)
				}
			}
			ev.Findings = append(ev.Findings, EvidenceValue{Key: rf.Key, Schema: rf.Schema, Value: value})
		}

		for _, pr := range handoff.ResolvePathCandidates(root, allPaths) {
			if pr.Status == handoff.PathResolved {
				ev.ResolvedPaths = append(ev.ResolvedPaths, pr.Resolved)
			} else {
				ev.UnresolvedPaths = append(ev.UnresolvedPaths, pr.Candidate)
			}
		}

		result = append(result, ev)
	}
	return result, nil
}

// renderFindingValue renders a finding's raw JSON value as a display string:
// text/line schemas unwrap to the plain string, everything else (e.g. the
// json_array_path_change array) is compacted JSON. Never errors — an
// unparseable value degrades to its raw bytes.
func renderFindingValue(schema string, raw json.RawMessage) string {
	unwrapped := unwrapOnce(raw)
	switch schema {
	case model.FindingSchemaNonemptyText, model.FindingSchemaOrderedLines:
		var s string
		if json.Unmarshal(unwrapped, &s) == nil {
			return s
		}
	}
	var buf bytes.Buffer
	if json.Compact(&buf, unwrapped) == nil {
		return buf.String()
	}
	return string(unwrapped)
}
