package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParsePlanManifest_ValidMinimal(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"version": 1,
		"goal": "fix the bug",
		"layers": [
			{"layer": 0, "policy": "all", "nodes": [
				{"id": "investigate", "template": "investigator", "instructions": "find it"}
			]}
		]
	}`)
	m, err := ParsePlanManifest(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m.Version != 1 || m.Goal != "fix the bug" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if len(m.Layers) != 1 || len(m.Layers[0].Nodes) != 1 {
		t.Fatalf("unexpected layers: %+v", m.Layers)
	}
}

func TestParsePlanManifest_RejectsUnknownTopLevelField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
	}{
		{"model", `{"version":1,"goal":"g","layers":[],"model":"opus"}`},
		{"effort", `{"version":1,"goal":"g","layers":[],"effort":"high"}`},
		{"finding_schemas", `{"version":1,"goal":"g","layers":[],"finding_schemas":[]}`},
		{"tools", `{"version":1,"goal":"g","layers":[],"tools":"emit_findings"}`},
		{"unknown_random", `{"version":1,"goal":"g","layers":[],"wat":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := ParsePlanManifest(json.RawMessage(tc.raw))
			if err == nil {
				t.Fatalf("expected error for unknown field %q, got nil", tc.name)
			}
		})
	}
}

func TestParsePlanManifest_RejectsUnknownLayerField(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"version": 1,
		"goal": "g",
		"layers": [
			{"layer": 0, "policy": "all", "nodes": [], "model": "opus"}
		]
	}`)
	if _, err := ParsePlanManifest(raw); err == nil {
		t.Fatalf("expected error for unknown layer field, got nil")
	}
}

func TestParsePlanManifest_RejectsUnknownNodeField(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"version": 1,
		"goal": "g",
		"layers": [
			{"layer": 0, "policy": "all", "nodes": [
				{"id": "a", "template": "t", "instructions": "i", "finding_schemas": []}
			]}
		]
	}`)
	if _, err := ParsePlanManifest(raw); err == nil {
		t.Fatalf("expected error for unknown node field, got nil")
	}
}

func TestParsePlanManifest_RejectsUnknownQuestionField(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"version": 1,
		"goal": "g",
		"layers": [
			{"layer": 0, "policy": "all", "nodes": [
				{"id": "a", "template": "t", "instructions": "i"}
			]}
		],
		"questions": [
			{"id": "q1", "question": "why?", "model": "opus"}
		]
	}`)
	if _, err := ParsePlanManifest(raw); err == nil {
		t.Fatalf("expected error for unknown question field, got nil")
	}
}

func TestParsePlanManifest_InvalidJSON(t *testing.T) {
	t.Parallel()
	_, err := ParsePlanManifest(json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParsePlanManifest_AcceptsQuestions(t *testing.T) {
	t.Parallel()
	raw := json.RawMessage(`{
		"version": 1,
		"goal": "g",
		"layers": [
			{"layer": 0, "policy": "all", "nodes": [
				{"id": "a", "template": "t", "instructions": "i"}
			]}
		],
		"questions": [
			{"id": "q1", "question": "should we do x?"}
		]
	}`)
	m, err := ParsePlanManifest(raw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(m.Questions) != 1 || m.Questions[0].ID != "q1" {
		t.Fatalf("unexpected questions: %+v", m.Questions)
	}
}

func TestHashManifest_DeterministicAndSensitive(t *testing.T) {
	t.Parallel()
	m1 := baseValidManifest("worker")
	m2 := baseValidManifest("worker")
	if HashManifest(m1) != HashManifest(m2) {
		t.Fatalf("expected identical manifests to hash the same")
	}
	m3 := baseValidManifest("worker")
	m3.Goal = "a different goal"
	if HashManifest(m1) == HashManifest(m3) {
		t.Fatalf("expected different manifests to hash differently")
	}
	if h := HashManifest(m1); len(h) != 64 || strings.ContainsAny(h, "GHIJKLMNOPQRSTUVWXYZ") {
		t.Fatalf("expected 64-char lowercase hex sha256, got %q", h)
	}
}
