package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

func TestPythonScriptService_CreateKindDefaultsToAgent(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Default Kind",
		Code: "x=1",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if script.Kind != "agent" {
		t.Errorf("Kind = %q, want agent (default)", script.Kind)
	}
}

func TestPythonScriptService_CreateKindExplicitAgent(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Explicit Agent",
		Kind: "agent",
		Code: "x=1",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if script.Kind != "agent" {
		t.Errorf("Kind = %q, want agent", script.Kind)
	}
}

func TestPythonScriptService_CreateKindInvalid(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Bad Kind",
		Kind: "garbage",
	})
	if err == nil {
		t.Fatal("Create() with kind=garbage: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "kind must be") {
		t.Errorf("error = %q, want to contain 'kind must be'", err.Error())
	}
}

func TestPythonScriptService_CreateKindTool_RequiresToolDescription(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:            "Tool No Desc",
		Kind:            "tool",
		ToolDescription: "",
	})
	if err == nil {
		t.Fatal("Create() kind=tool without tool_description: expected error, got nil")
	}
	if !strings.Contains(err.Error(), "tool_description") {
		t.Errorf("error = %q, want to contain 'tool_description'", err.Error())
	}
}

func TestPythonScriptService_CreateKindTool_InvalidInputSchema(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	cases := []struct {
		name   string
		schema string
	}{
		{"not json", "not-json"},
		{"bad type", `{"type":"not-a-type"}`},
		{"invalid draft2020", `{"type":123}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
				Name:            "Tool Bad Schema " + tc.name,
				Kind:            "tool",
				ToolDescription: "does x",
				InputSchema:     tc.schema,
			})
			if err == nil {
				t.Fatalf("Create() with schema %q: expected error, got nil", tc.schema)
			}
			if !strings.Contains(err.Error(), "input_schema") {
				t.Errorf("error = %q, want to contain 'input_schema'", err.Error())
			}
		})
	}
}

func TestPythonScriptService_CreateKindTool_ValidInputSchema(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	cases := []struct{ name, schema string }{
		{"empty object", "{}"},
		{"typed object", `{"type":"object","properties":{"x":{"type":"string"}}}`},
		{"empty string defaults to object", ""},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := &types.PythonScriptCreateRequest{
				Name:            "Tool Valid Schema " + strings.ReplaceAll(tc.name, " ", "_"),
				Kind:            "tool",
				ToolDescription: "does x",
				InputSchema:     tc.schema,
				TimeoutSec:      i + 1,
			}
			_, err := svc.Create(projectID, req)
			if err != nil {
				t.Errorf("Create() schema %q: unexpected error: %v", tc.schema, err)
			}
		})
	}
}

func TestPythonScriptService_CreateKindTool_TimeoutDefault(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:            "Tool Default Timeout",
		Kind:            "tool",
		ToolDescription: "does x",
		TimeoutSec:      0,
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if script.TimeoutSec != 30 {
		t.Errorf("TimeoutSec = %d, want 30 (default)", script.TimeoutSec)
	}
}

func TestPythonScriptService_CreateKindTool_TimeoutRange(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	cases := []struct {
		name    string
		timeout int
		wantErr bool
	}{
		{"min boundary", 1, false},
		{"max boundary", 600, false},
		{"below min", -1, true},
		{"above max", 601, true},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
				Name:            "ToolTimeout",
				Kind:            "tool",
				ToolDescription: "does x",
				TimeoutSec:      tc.timeout,
				// Use unique names to avoid UNIQUE conflicts
				Code: strings.Repeat("x", i),
			})
			// Rename to avoid collision
			_, err = svc.Create(projectID, &types.PythonScriptCreateRequest{
				Name:            "TT" + tc.name,
				Kind:            "tool",
				ToolDescription: "does x",
				TimeoutSec:      tc.timeout,
			})
			if tc.wantErr && err == nil {
				t.Errorf("Create(timeout=%d): expected error, got nil", tc.timeout)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("Create(timeout=%d): unexpected error: %v", tc.timeout, err)
			}
			if tc.wantErr && err != nil && !strings.Contains(err.Error(), "timeout_sec") {
				t.Errorf("Create(timeout=%d) error = %q, want to contain 'timeout_sec'", tc.timeout, err.Error())
			}
		})
	}
}

func TestPythonScriptService_CreateKindTool_Success(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	script, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name:            "My Tool",
		Kind:            "tool",
		Description:     "A useful tool",
		ToolDescription: "Does something useful for the agent",
		InputSchema:     `{"type":"object","properties":{"q":{"type":"string"}},"required":["q"]}`,
		TimeoutSec:      45,
	})
	if err != nil {
		t.Fatalf("Create() tool: %v", err)
	}
	if script.Kind != "tool" {
		t.Errorf("Kind = %q, want tool", script.Kind)
	}
	if script.ToolDescription != "Does something useful for the agent" {
		t.Errorf("ToolDescription = %q", script.ToolDescription)
	}
	if script.TimeoutSec != 45 {
		t.Errorf("TimeoutSec = %d, want 45", script.TimeoutSec)
	}
	if !strings.HasPrefix(script.ID, "ps-") {
		t.Errorf("ID = %q, want ps- prefix", script.ID)
	}
}

