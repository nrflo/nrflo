package apirun

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"be/internal/model"
	"be/internal/spawner/apirun/provider"
)

// fakeManifestProvider implements ManifestProvider for registry tests.
type fakeManifestProvider struct {
	specs    []provider.ToolSpec
	handlers map[string]ToolHandler
}

func (p *fakeManifestProvider) Specs() []provider.ToolSpec { return p.specs }

func (p *fakeManifestProvider) Handler(name string) (ToolHandler, bool) {
	h, ok := p.handlers[name]
	return h, ok
}

// newFakeManifestProvider builds a provider with the given tool names.
func newFakeManifestProvider(names ...string) *fakeManifestProvider {
	p := &fakeManifestProvider{handlers: make(map[string]ToolHandler)}
	for _, n := range names {
		spec := provider.ToolSpec{Name: n, Description: "manifest " + n, InputSchema: json.RawMessage(`{}`)}
		p.specs = append(p.specs, spec)
		n := n
		p.handlers[n] = stubHandler{name: n}
	}
	return p
}

// manifestStub is a ToolHandler that also implements the Spec method.
type manifestStub struct{ name string }

func (m manifestStub) Spec() provider.ToolSpec {
	return provider.ToolSpec{Name: m.name, Description: "manifest stub", InputSchema: json.RawMessage(`{}`)}
}

func (m manifestStub) Invoke(_ context.Context, _ ToolEnv, _ json.RawMessage) (string, bool, error) {
	return "manifest-ok", false, nil
}

// TestResolveRegistry_ManifestCollision_WithBuiltin verifies that a manifest tool
// whose name matches a builtin returns an error containing "collides with builtin".
func TestResolveRegistry_ManifestCollision_WithBuiltin(t *testing.T) {
	// "findings_add" is a builtin name.
	mp := newFakeManifestProvider("findings_add")
	_, _, err := ResolveRegistry("findings_add", defaultBuiltins(), nil, httpFactoryStub, mp)
	if err == nil {
		t.Fatalf("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "collides with builtin") {
		t.Errorf("error = %q, want substring 'collides with builtin'", err.Error())
	}
}

// TestResolveRegistry_ManifestCollision_WithHTTPDef verifies that an HTTP tool def
// whose name matches a manifest tool returns an error containing "collides with manifest tool".
func TestResolveRegistry_ManifestCollision_WithHTTPDef(t *testing.T) {
	mp := newFakeManifestProvider("my_manifest_tool")
	httpDefs := []*model.ToolDefinition{{Name: "my_manifest_tool"}}
	_, _, err := ResolveRegistry("my_manifest_tool", defaultBuiltins(), httpDefs, httpFactoryStub, mp)
	if err == nil {
		t.Fatalf("expected collision error, got nil")
	}
	if !strings.Contains(err.Error(), "collides with manifest tool") {
		t.Errorf("error = %q, want substring 'collides with manifest tool'", err.Error())
	}
}

// TestResolveRegistry_ManifestUniqueToolPresent verifies that a manifest tool with
// a unique name is included in both specs and the registry.
func TestResolveRegistry_ManifestUniqueToolPresent(t *testing.T) {
	mp := newFakeManifestProvider("lookup_sku")
	specs, reg, err := ResolveRegistry("lookup_sku", defaultBuiltins(), nil, httpFactoryStub, mp)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if _, ok := reg["lookup_sku"]; !ok {
		t.Errorf("lookup_sku not in registry; keys=%v", sortedKeys(reg))
	}
	found := false
	for _, s := range specs {
		if s.Name == "lookup_sku" {
			found = true
		}
	}
	if !found {
		t.Errorf("lookup_sku not in specs")
	}
}

// TestResolveRegistry_StarIncludesManifestTools verifies that "*" matches builtins
// plus all manifest tools in addition to HTTP defs.
func TestResolveRegistry_StarIncludesManifestTools(t *testing.T) {
	mp := newFakeManifestProvider("manifest_alpha", "manifest_beta")
	httpDefs := []*model.ToolDefinition{{Name: "http_gamma"}}
	_, reg, err := ResolveRegistry("*", defaultBuiltins(), httpDefs, httpFactoryStub, mp)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	for _, name := range []string{"manifest_alpha", "manifest_beta", "http_gamma", "findings_add"} {
		if _, ok := reg[name]; !ok {
			t.Errorf("expected %q in registry, got %v", name, sortedKeys(reg))
		}
	}
}

// TestResolveRegistry_ManifestToolSkippedIfHandlerMissing verifies that a manifest
// tool whose Handler() returns false is silently skipped (not an error).
func TestResolveRegistry_ManifestToolSkippedIfHandlerMissing(t *testing.T) {
	// Provider reports "ghost_tool" in Specs but returns false for Handler.
	mp := &fakeManifestProvider{
		specs:    []provider.ToolSpec{{Name: "ghost_tool", InputSchema: json.RawMessage(`{}`)}},
		handlers: map[string]ToolHandler{},
	}
	// With only "ghost_tool" in the CSV and handler missing, no tools match.
	_, _, err := ResolveRegistry("ghost_tool", defaultBuiltins(), nil, httpFactoryStub, mp)
	if err == nil {
		t.Fatalf("expected no-match error when handler is missing, got nil")
	}
	if !strings.Contains(err.Error(), "no tools matched") {
		t.Errorf("error = %q, want 'no tools matched'", err.Error())
	}
}

// TestResolveRegistry_NilManifest_NoRegression verifies that nil manifestProvider
// preserves existing behaviour (builtins + HTTP only).
func TestResolveRegistry_NilManifest_NoRegression(t *testing.T) {
	httpDefs := []*model.ToolDefinition{{Name: "http_tool"}}
	specs, reg, err := ResolveRegistry("http_tool,findings_add", defaultBuiltins(), httpDefs, httpFactoryStub, nil)
	if err != nil {
		t.Fatalf("ResolveRegistry: %v", err)
	}
	if len(reg) != 2 {
		t.Errorf("registry size = %d, want 2", len(reg))
	}
	if len(specs) != 2 {
		t.Errorf("specs size = %d, want 2", len(specs))
	}
}
