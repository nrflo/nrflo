package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"be/internal/clock"
	"be/internal/types"
)

// TestBuildAPIProvider_CustomProvider_ReturnsProviderNamedAfterRow verifies
// the default case resolves a registered+enabled custom_providers row and
// reports its own stored name (Rule 6: registry lookup, no name-check).
func TestBuildAPIProvider_CustomProvider_ReturnsProviderNamedAfterRow(t *testing.T) {
	t.Parallel()
	pool, projectID := setupAPIProviderTest(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cpSvc := NewCustomProviderService(pool, clk)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name: "local-ollama", BaseURL: "http://localhost:11434/v1",
	}); err != nil {
		t.Fatalf("create custom provider: %v", err)
	}

	got, err := BuildAPIProvider(context.Background(), pool, clk, "local-ollama", projectID)
	if err != nil {
		t.Fatalf("BuildAPIProvider: %v", err)
	}
	if got == nil {
		t.Fatal("BuildAPIProvider = nil, want non-nil provider")
	}
	if got.Name() != "local-ollama" {
		t.Errorf("provider.Name() = %q, want local-ollama", got.Name())
	}
}

// TestBuildAPIProvider_CustomProvider_Disabled_ReturnsError verifies a
// disabled custom provider row is rejected rather than silently built.
func TestBuildAPIProvider_CustomProvider_Disabled_ReturnsError(t *testing.T) {
	t.Parallel()
	pool, projectID := setupAPIProviderTest(t)
	clk := clock.NewTest(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	cpSvc := NewCustomProviderService(pool, clk)
	if _, err := cpSvc.Create(types.CustomProviderCreateRequest{
		Name: "local-lmstudio", BaseURL: "http://localhost:1234/v1",
	}); err != nil {
		t.Fatalf("create custom provider: %v", err)
	}
	disabled := false
	if _, err := cpSvc.Update("local-lmstudio", types.CustomProviderUpdateRequest{Enabled: &disabled}); err != nil {
		t.Fatalf("disable custom provider: %v", err)
	}

	got, err := BuildAPIProvider(context.Background(), pool, clk, "local-lmstudio", projectID)
	if err == nil {
		t.Error("BuildAPIProvider(disabled custom provider) = nil error, want error")
	}
	if got != nil {
		t.Error("BuildAPIProvider(disabled custom provider) = non-nil provider, want nil")
	}
	if err != nil && !strings.Contains(err.Error(), "unknown or disabled provider") {
		t.Errorf("error = %v, want mention of unknown or disabled provider", err)
	}
}

// TestBuildAPIProvider_CustomProvider_NotRegistered_ReturnsError verifies a
// name that isn't a builtin and has no custom_providers row still hits the
// existing unknown-provider error path.
func TestBuildAPIProvider_CustomProvider_NotRegistered_ReturnsError(t *testing.T) {
	t.Parallel()
	pool, projectID := setupAPIProviderTest(t)

	got, err := BuildAPIProvider(context.Background(), pool, clock.Real(), "never-registered", projectID)
	if err == nil {
		t.Error("BuildAPIProvider(unregistered) = nil error, want error")
	}
	if got != nil {
		t.Error("BuildAPIProvider(unregistered) = non-nil provider, want nil")
	}
}
