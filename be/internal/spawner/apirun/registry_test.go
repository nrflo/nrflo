package apirun

import (
	"context"
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/spawner/apirun/provider"
)

// stubHandler is a minimal ToolHandler used to populate fake builtin/HTTP
// pools for registry resolution tests.
type stubHandler struct {
	name string
}

func (s stubHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: s.name, Description: "stub", InputSchema: json.RawMessage(`{}`)}
}

func (s stubHandler) Invoke(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, bool, error) {
	return "ok", false, nil
}

func newStubBuiltins(names ...string) map[string]ToolHandler {
	out := make(map[string]ToolHandler, len(names))
	for _, n := range names {
		out[n] = stubHandler{name: n}
	}
	return out
}

func defaultBuiltins() map[string]ToolHandler {
	return newStubBuiltins(
		"findings_add", "findings_add_bulk", "findings_append", "findings_append_bulk",
		"findings_get", "findings_delete",
		"project_findings_add", "project_findings_get",
		"agent_fail", "agent_continue", "agent_callback", "agent_context_update",
		"workflow_skip",
	)
}

func httpFactoryStub(def *model.ToolDefinition) ToolHandler {
	return stubHandler{name: def.Name}
}

func sortedKeys(m Registry) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func TestResolveRegistry_EmptyCSV(t *testing.T) {
	specs, reg, err := ResolveRegistry("", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if len(reg) != 0 {
		t.Errorf("registry = %d entries, want 0", len(reg))
	}
	if len(specs) != 0 {
		t.Errorf("specs = %d entries, want 0", len(specs))
	}
}

func TestResolveRegistry_FindingsGlob(t *testing.T) {
	_, reg, err := ResolveRegistry("findings_*", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	want := []string{
		"findings_add", "findings_add_bulk", "findings_append", "findings_append_bulk",
		"findings_delete", "findings_get",
	}
	got := sortedKeys(reg)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("registry keys = %v, want %v", got, want)
	}
}

func TestResolveRegistry_AgentGlobAndExact(t *testing.T) {
	_, reg, err := ResolveRegistry("agent_*,workflow_skip", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	want := []string{
		"agent_callback", "agent_context_update", "agent_continue", "agent_fail",
		"workflow_skip",
	}
	got := sortedKeys(reg)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("registry keys = %v, want %v", got, want)
	}
}

func TestResolveRegistry_StarMatchesAll(t *testing.T) {
	httpDefs := []*model.ToolDefinition{{Name: "lookup_sku"}}
	_, reg, err := ResolveRegistry("*", defaultBuiltins(), httpDefs, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if len(reg) != len(defaultBuiltins())+1 {
		t.Errorf("registry size = %d, want %d", len(reg), len(defaultBuiltins())+1)
	}
	if _, ok := reg["lookup_sku"]; !ok {
		t.Errorf("expected lookup_sku in registry, got %v", sortedKeys(reg))
	}
}

func TestResolveRegistry_HTTPExactMatch(t *testing.T) {
	httpDefs := []*model.ToolDefinition{{Name: "lookup_sku"}, {Name: "search_db"}}
	_, reg, err := ResolveRegistry("lookup_sku", defaultBuiltins(), httpDefs, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if len(reg) != 1 {
		t.Fatalf("registry = %v, want only lookup_sku", sortedKeys(reg))
	}
	if _, ok := reg["lookup_sku"]; !ok {
		t.Errorf("expected lookup_sku, got %v", sortedKeys(reg))
	}
}

func TestResolveRegistry_NoMatchIsConfigError(t *testing.T) {
	_, _, err := ResolveRegistry("lookup_sku", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err == nil {
		t.Fatalf("expected config error, got nil")
	}
	if !strings.Contains(err.Error(), "no tools matched") {
		t.Errorf("error = %q, want substring 'no tools matched'", err.Error())
	}
}

func TestResolveRegistry_BuiltinHTTPCollision(t *testing.T) {
	httpDefs := []*model.ToolDefinition{{Name: "findings_add"}}
	_, _, err := ResolveRegistry("findings_add", defaultBuiltins(), httpDefs, httpFactoryStub, nil)
	if err == nil {
		t.Fatalf("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "collides with builtin") {
		t.Errorf("error = %q, want substring 'collides with builtin'", err.Error())
	}
}

func TestResolveRegistry_DedupAcrossPatterns(t *testing.T) {
	// "findings_add" appears via both the exact match and the glob; dedup
	// must keep one entry.
	_, reg, err := ResolveRegistry("findings_add,findings_*", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if len(reg) != 6 {
		t.Errorf("registry size = %d, want 6 (six findings builtins)", len(reg))
	}
}

func TestResolveRegistry_WhitespaceTrimmed(t *testing.T) {
	_, reg, err := ResolveRegistry(" findings_add , workflow_skip ", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if _, ok := reg["findings_add"]; !ok {
		t.Errorf("missing findings_add")
	}
	if _, ok := reg["workflow_skip"]; !ok {
		t.Errorf("missing workflow_skip")
	}
}

func TestResolveRegistry_SpecsMirrorRegistry(t *testing.T) {
	specs, reg, err := ResolveRegistry("findings_*", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if len(specs) != len(reg) {
		t.Errorf("specs len = %d, registry len = %d", len(specs), len(reg))
	}
	specNames := map[string]bool{}
	for _, s := range specs {
		specNames[s.Name] = true
	}
	for name := range reg {
		if !specNames[name] {
			t.Errorf("registry has %q but specs do not", name)
		}
	}
}

// TestResolveRegistry_DotPatternMismatch documents that the dotted form
// `findings.*` referenced in the ticket and apirun/CLAUDE.md does NOT match
// the actual builtin names, which use underscores (`findings_add` etc.).
// The implementation interprets `prefix.*` as a literal prefix `prefix.`
// followed by anything, so the dotted form is not a wildcard separator.
// Users must write `findings_*` for the underscore-style names. Recorded as
// a production/docs mismatch in be_production_bugs.
func TestResolveRegistry_DotPatternMismatch(t *testing.T) {
	_, _, err := ResolveRegistry("findings.*", defaultBuiltins(), nil, httpFactoryStub, nil)
	if err == nil {
		t.Fatalf("expected `findings.*` to NOT match any builtin (names use underscores), but ResolveRegistry succeeded")
	}
	if !strings.Contains(err.Error(), "no tools matched pattern") {
		t.Errorf("err = %q, want 'no tools matched pattern'", err.Error())
	}
}

func TestResolveRegistry_NilHTTPDefSkipped(t *testing.T) {
	httpDefs := []*model.ToolDefinition{nil, {Name: ""}, {Name: "lookup_sku"}}
	_, reg, err := ResolveRegistry("lookup_sku", defaultBuiltins(), httpDefs, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if _, ok := reg["lookup_sku"]; !ok {
		t.Errorf("expected lookup_sku to resolve, got %v", sortedKeys(reg))
	}
}
