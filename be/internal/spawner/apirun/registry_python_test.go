package apirun

import (
	"strings"
	"testing"

	"be/internal/model"
)

// newPythonStubs builds a slice of stubHandler ToolHandlers for use as python handlers.
func newPythonStubs(names ...string) []ToolHandler {
	out := make([]ToolHandler, 0, len(names))
	for _, n := range names {
		out = append(out, stubHandler{name: n})
	}
	return out
}

// TestResolveRegistry_PythonTool_ResolvesWithEmptyBuiltins verifies that a python
// tool handler resolves correctly when the builtins map is empty.
func TestResolveRegistry_PythonTool_ResolvesWithEmptyBuiltins(t *testing.T) {
	handlers := newPythonStubs("lookup_customer")
	_, reg, err := ResolveRegistry("lookup_customer", map[string]ToolHandler{}, handlers, nil, httpFactoryStub)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if _, ok := reg["lookup_customer"]; !ok {
		t.Errorf("lookup_customer not in registry, got %v", sortedKeys(reg))
	}
	if len(reg) != 1 {
		t.Errorf("registry size = %d, want 1", len(reg))
	}
}

// TestResolveRegistry_PythonHTTPCollision verifies that an HTTP tool with the same
// name as a python tool returns "collides with python tool".
func TestResolveRegistry_PythonHTTPCollision(t *testing.T) {
	handlers := newPythonStubs("lookup_sku")
	httpDefs := []*model.ToolDefinition{{Name: "lookup_sku"}}
	_, _, err := ResolveRegistry("lookup_sku", defaultBuiltins(), handlers, httpDefs, httpFactoryStub)
	if err == nil {
		t.Fatalf("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "collides with python tool") {
		t.Errorf("error = %q, want substring 'collides with python tool'", err.Error())
	}
}

// TestResolveRegistry_BuiltinPythonCollision verifies that a python tool with the
// same name as a builtin returns "collides with builtin".
func TestResolveRegistry_BuiltinPythonCollision(t *testing.T) {
	handlers := newPythonStubs("findings_add")
	_, _, err := ResolveRegistry("findings_add", defaultBuiltins(), handlers, nil, httpFactoryStub)
	if err == nil {
		t.Fatalf("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "collides with builtin") {
		t.Errorf("error = %q, want substring 'collides with builtin'", err.Error())
	}
}

// TestResolveRegistry_PythonPrefixGlob verifies that prefix* pattern matching
// works against python tool names, including partial matches.
func TestResolveRegistry_PythonPrefixGlob(t *testing.T) {
	handlers := newPythonStubs("git_commit", "git_push", "search_db")
	_, reg, err := ResolveRegistry("git_*", map[string]ToolHandler{}, handlers, nil, httpFactoryStub)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if _, ok := reg["git_commit"]; !ok {
		t.Errorf("git_commit missing from registry")
	}
	if _, ok := reg["git_push"]; !ok {
		t.Errorf("git_push missing from registry")
	}
	if _, ok := reg["search_db"]; ok {
		t.Errorf("search_db should not be in registry (not matched by git_*)")
	}
	if len(reg) != 2 {
		t.Errorf("registry size = %d, want 2", len(reg))
	}
}

// TestResolveRegistry_PythonStarMatchesAll verifies that "*" selects python tools
// together with builtins when both are present.
func TestResolveRegistry_PythonStarMatchesAll(t *testing.T) {
	handlers := newPythonStubs("custom_tool")
	_, reg, err := ResolveRegistry("*", defaultBuiltins(), handlers, nil, httpFactoryStub)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if _, ok := reg["custom_tool"]; !ok {
		t.Errorf("custom_tool not in registry")
	}
	wantSize := len(defaultBuiltins()) + 1
	if len(reg) != wantSize {
		t.Errorf("registry size = %d, want %d", len(reg), wantSize)
	}
}
