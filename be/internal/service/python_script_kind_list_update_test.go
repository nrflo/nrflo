package service

import (
	"strings"
	"testing"

	"be/internal/types"
)

func TestPythonScriptService_ListByKind(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	for _, req := range []*types.PythonScriptCreateRequest{
		{Name: "Agent A", Kind: "agent"},
		{Name: "Agent B", Kind: "agent"},
		{Name: "Tool X", Kind: "tool", ToolDescription: "x"},
	} {
		if _, err := svc.Create(projectID, req); err != nil {
			t.Fatalf("Create(%s): %v", req.Name, err)
		}
	}

	agents, err := svc.ListByKind(projectID, "agent")
	if err != nil {
		t.Fatalf("ListByKind(agent): %v", err)
	}
	if len(agents) != 2 {
		t.Errorf("ListByKind(agent) = %d, want 2", len(agents))
	}

	tools, err := svc.ListByKind(projectID, "tool")
	if err != nil {
		t.Fatalf("ListByKind(tool): %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("ListByKind(tool) = %d, want 1", len(tools))
	}
}

func TestPythonScriptService_ListTools(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	if _, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Tool Only", Kind: "tool", ToolDescription: "x",
	}); err != nil {
		t.Fatalf("Create tool: %v", err)
	}
	if _, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Agent Only", Kind: "agent",
	}); err != nil {
		t.Fatalf("Create agent: %v", err)
	}

	tools, err := svc.ListTools(projectID)
	if err != nil {
		t.Fatalf("ListTools(): %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("ListTools() = %d, want 1", len(tools))
	}
	if len(tools) > 0 && tools[0].Kind != "tool" {
		t.Errorf("ListTools() returned kind=%q, want tool", tools[0].Kind)
	}
}

func TestPythonScriptService_UpdateInputSchemaValidation(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	tool, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Tool Update Schema", Kind: "tool", ToolDescription: "x",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	invalidSchema := `{"type":"not-a-type"}`
	err = svc.Update(projectID, tool.ID, &types.PythonScriptUpdateRequest{
		InputSchema: &invalidSchema,
	})
	if err == nil {
		t.Error("Update() with invalid schema: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "input_schema") {
		t.Errorf("error = %q, want to contain 'input_schema'", err.Error())
	}
}

func TestPythonScriptService_UpdateTimeoutRange(t *testing.T) {
	svc, projectID := setupPythonScriptSvc(t)

	tool, err := svc.Create(projectID, &types.PythonScriptCreateRequest{
		Name: "Tool Timeout Update", Kind: "tool", ToolDescription: "x",
	})
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}

	invalid := 601
	err = svc.Update(projectID, tool.ID, &types.PythonScriptUpdateRequest{
		TimeoutSec: &invalid,
	})
	if err == nil {
		t.Error("Update() with timeout=601: expected error, got nil")
	}
	if err != nil && !strings.Contains(err.Error(), "timeout_sec") {
		t.Errorf("error = %q, want to contain 'timeout_sec'", err.Error())
	}

	valid := 300
	if err := svc.Update(projectID, tool.ID, &types.PythonScriptUpdateRequest{
		TimeoutSec: &valid,
	}); err != nil {
		t.Errorf("Update() with timeout=300: unexpected error: %v", err)
	}
}
