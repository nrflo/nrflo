package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
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

// cwdTestServer serves GET /api/v1/projects with the given roots and echoes the
// X-Project header of any /api/v1/workflows call into *sawProject.
func cwdTestServer(t *testing.T, projects []projRoot, sawProject *string) *nrfloHTTPClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/projects":
			body := `{"projects":[`
			for i, p := range projects {
				if i > 0 {
					body += ","
				}
				body += `{"id":"` + p.ID + `","root_path":"` + p.RootPath + `"}`
			}
			body += `]}`
			_, _ = w.Write([]byte(body))
		case "/api/v1/workflows":
			*sawProject = r.Header.Get("X-Project")
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	return &nrfloHTTPClient{base: srv.URL, token: "tok", defaultProject: "envdefault", hc: srv.Client()}
}

func TestResolveProject_CwdBeatsEnvDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir) // proxy cwd = a project's root_path
	var saw string
	c := cwdTestServer(t, []projRoot{{ID: "cwdproj", RootPath: dir}}, &saw)

	if _, err := callExternalTool(context.Background(), c, "list_workflows", nil); err != nil {
		t.Fatalf("list_workflows: %v", err)
	}
	if saw != "cwdproj" {
		t.Errorf("X-Project = %q, want cwdproj (cwd auto-detect should beat NRFLO_PROJECT=envdefault)", saw)
	}
}

func TestResolveProject_ExplicitArgBeatsCwd(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var saw string
	c := cwdTestServer(t, []projRoot{{ID: "cwdproj", RootPath: dir}}, &saw)

	if _, err := callExternalTool(context.Background(), c, "list_workflows", []byte(`{"project":"explicit"}`)); err != nil {
		t.Fatalf("list_workflows: %v", err)
	}
	if saw != "explicit" {
		t.Errorf("X-Project = %q, want explicit (arg should beat cwd)", saw)
	}
}

func TestResolveProject_NoCwdMatchUsesEnvDefault(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var saw string
	// Projects list contains nothing matching the cwd → falls back to NRFLO_PROJECT.
	c := cwdTestServer(t, []projRoot{{ID: "other", RootPath: "/somewhere/else"}}, &saw)

	if _, err := callExternalTool(context.Background(), c, "list_workflows", nil); err != nil {
		t.Fatalf("list_workflows: %v", err)
	}
	if saw != "envdefault" {
		t.Errorf("X-Project = %q, want envdefault (no cwd match → NRFLO_PROJECT)", saw)
	}
}

func TestCwdProject_CachedOncePerProcess(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	var saw string
	c := cwdTestServer(t, []projRoot{{ID: "cwdproj", RootPath: dir}}, &saw)
	// Two resolutions; the GET /api/v1/projects lookup must run at most once.
	a := c.resolveProject(context.Background(), "")
	b := c.resolveProject(context.Background(), "")
	if a != "cwdproj" || b != "cwdproj" {
		t.Fatalf("resolved %q then %q, want cwdproj both", a, b)
	}
}
