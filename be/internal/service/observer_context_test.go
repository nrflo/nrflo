package service

import (
	"testing"

	"be/internal/clock"
	"be/internal/repo"
)

// --- AssembleDynamicContext dispatch ---

func TestAssembleDynamicContext_UnknownScope(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupObserverTestEnv(t)

	_, err := AssembleDynamicContext(svc, "badscope", "proj1", "")
	if err == nil {
		t.Fatal("expected error for unknown scope, got nil")
	}
}

func TestAssembleDynamicContext_GlobalScope_NonEmpty(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupObserverTestEnv(t)

	out, err := AssembleDynamicContext(svc, "global", "", "")
	if err != nil {
		t.Fatalf("AssembleDynamicContext(global): %v", err)
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
	if !strContains(out, "## Global Context") {
		t.Errorf("missing '## Global Context' header in: %q", truncate(out, 200))
	}
}

func TestAssembleDynamicContext_GlobalScope_ContainsProject(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupObserverTestEnv(t)

	out, err := AssembleDynamicContext(svc, "global", "", "")
	if err != nil {
		t.Fatalf("AssembleDynamicContext(global): %v", err)
	}
	// proj1 was inserted by setupObserverTestEnv
	if !strContains(out, "proj1") {
		t.Errorf("expected project 'proj1' in output: %q", truncate(out, 300))
	}
}

func TestAssembleDynamicContext_ProjectScope_Header(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupObserverTestEnv(t)

	out, err := AssembleDynamicContext(svc, "project", "proj1", "")
	if err != nil {
		t.Fatalf("AssembleDynamicContext(project): %v", err)
	}
	if !strContains(out, "## Project Context") {
		t.Errorf("missing '## Project Context': %q", truncate(out, 200))
	}
	if !strContains(out, "proj1") {
		t.Errorf("missing project ID")
	}
}

func TestAssembleDynamicContext_WorkflowScope_Header(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupObserverTestEnv(t)

	out, err := AssembleDynamicContext(svc, "workflow", "proj1", "wf1")
	if err != nil {
		t.Fatalf("AssembleDynamicContext(workflow): %v", err)
	}
	if !strContains(out, "## Workflow Context") {
		t.Errorf("missing '## Workflow Context': %q", truncate(out, 200))
	}
}

func TestAssembleDynamicContext_WorkflowScope_MissingWorkflowNoError(t *testing.T) {
	t.Parallel()
	_, svc, _ := setupObserverTestEnv(t)

	out, err := AssembleDynamicContext(svc, "workflow", "proj1", "does-not-exist")
	if err != nil {
		t.Fatalf("expected no error for unknown workflow, got: %v", err)
	}
	if !strContains(out, "## Workflow Context") {
		t.Errorf("missing header: %q", truncate(out, 200))
	}
}

// --- Workflow observer columns round-trip ---

func TestWorkflowObserverColumns_Defaults(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)

	wfRepo := repo.NewWorkflowRepo(pool, clock.Real())
	wf, err := wfRepo.Get("proj1", "wf1")
	if err != nil {
		t.Fatalf("Get workflow: %v", err)
	}
	if wf.ObserverContext != "" {
		t.Errorf("ObserverContext default = %q, want empty", wf.ObserverContext)
	}
	if wf.ObserverProvider.Valid {
		t.Errorf("ObserverProvider default should be NULL")
	}
	if wf.ObserverModel.Valid {
		t.Errorf("ObserverModel default should be NULL")
	}
}

func TestWorkflowObserverColumns_RoundTrip(t *testing.T) {
	t.Parallel()
	pool, _, _ := setupObserverTestEnv(t)

	if _, err := pool.Exec(
		`UPDATE workflows SET observer_context=?,observer_provider=?,observer_model=? WHERE project_id=? AND id=?`,
		"my-ctx", "claude", "opus",
		"proj1", "wf1",
	); err != nil {
		t.Fatalf("update: %v", err)
	}

	wfRepo := repo.NewWorkflowRepo(pool, clock.Real())
	wf, err := wfRepo.Get("proj1", "wf1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if wf.ObserverContext != "my-ctx" {
		t.Errorf("ObserverContext = %q, want my-ctx", wf.ObserverContext)
	}
	if !wf.ObserverProvider.Valid || wf.ObserverProvider.String != "claude" {
		t.Errorf("ObserverProvider = %v, want {claude, true}", wf.ObserverProvider)
	}
	if !wf.ObserverModel.Valid || wf.ObserverModel.String != "opus" {
		t.Errorf("ObserverModel = %v, want {opus, true}", wf.ObserverModel)
	}
}
