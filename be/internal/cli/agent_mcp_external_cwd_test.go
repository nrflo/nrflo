package cli

import (
	"context"
	"testing"

	"be/internal/service"
)

func TestMatchProjectByCwd(t *testing.T) {
	t.Parallel()
	projects := []projRoot{
		{ID: "acme", RootPath: "/work/acme"},
		{ID: "acme-sub", RootPath: "/work/acme/sub"}, // nested → longest-prefix wins
		{ID: "blog", RootPath: "/work/blog"},
		{ID: "noroot", RootPath: ""},     // skipped
		{ID: "rootslash", RootPath: "/"}, // skipped (catch-all guard)
	}
	cases := []struct {
		cwd, want string
	}{
		{"/work/acme", "acme"},              // exact
		{"/work/acme/x/y", "acme"},          // ancestor
		{"/work/acme/sub", "acme-sub"},      // nested exact, longest wins
		{"/work/acme/sub/deep", "acme-sub"}, // nested ancestor, longest wins
		{"/work/acmefoo", ""},               // segment boundary: NOT under /work/acme
		{"/elsewhere", ""},                  // no match
		{"/", ""},                           // cwd root matches nothing real
	}
	for _, tc := range cases {
		if got := matchProjectByCwd(tc.cwd, projects); got != tc.want {
			t.Errorf("matchProjectByCwd(%q) = %q, want %q", tc.cwd, got, tc.want)
		}
	}
}

// The following three tests cover case 2: the project resolved at SESSION
// CREATION (resolveSessionProject, exercised via openConsoleSession) — cwd
// match beats NRFLO_PROJECT, no cwd match falls back to NRFLO_PROJECT, and
// neither set falls back to the hidden global project. Assertions read the
// X-Project header of the create-session request.

func TestResolveSessionProject_CwdBeatsEnvDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // proxy cwd = a project's root_path
	f := newFakeConsoleServer(t)
	f.projects = []projRoot{{ID: "cwdproj", RootPath: dir}}
	c := &nrfloHTTPClient{base: f.url(), serviceToken: f.serviceToken, defaultProject: "envdefault", hc: f.srv.Client()}

	if err := c.openConsoleSession(context.Background()); err != nil {
		t.Fatalf("openConsoleSession: %v", err)
	}
	if got := f.createReqs[0].project; got != "cwdproj" {
		t.Errorf("X-Project = %q, want cwdproj (cwd auto-detect should beat NRFLO_PROJECT=envdefault)", got)
	}
}

func TestResolveSessionProject_NoCwdMatchUsesEnvDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	f := newFakeConsoleServer(t)
	// Projects list contains nothing matching the cwd → falls back to NRFLO_PROJECT.
	f.projects = []projRoot{{ID: "other", RootPath: "/somewhere/else"}}
	c := &nrfloHTTPClient{base: f.url(), serviceToken: f.serviceToken, defaultProject: "envdefault", hc: f.srv.Client()}

	if err := c.openConsoleSession(context.Background()); err != nil {
		t.Fatalf("openConsoleSession: %v", err)
	}
	if got := f.createReqs[0].project; got != "envdefault" {
		t.Errorf("X-Project = %q, want envdefault (no cwd match → NRFLO_PROJECT)", got)
	}
}

func TestResolveSessionProject_FallsBackToGlobal(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	f := newFakeConsoleServer(t)
	// No cwd match and no NRFLO_PROJECT default → the hidden global project.
	c := &nrfloHTTPClient{base: f.url(), serviceToken: f.serviceToken, hc: f.srv.Client()}

	if err := c.openConsoleSession(context.Background()); err != nil {
		t.Fatalf("openConsoleSession: %v", err)
	}
	if got := f.createReqs[0].project; got != service.GlobalProjectID {
		t.Errorf("X-Project = %q, want %q (global fallback)", got, service.GlobalProjectID)
	}
}
