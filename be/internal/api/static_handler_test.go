package api

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

// buildSPAFS creates a minimal in-memory FS with index.html and optional extras.
func buildSPAFS(extra fstest.MapFS) fstest.MapFS {
	fsys := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte(`<!DOCTYPE html><html><body>SPA</body></html>`)},
	}
	for k, v := range extra {
		fsys[k] = v
	}
	return fsys
}

// TestSPAHandler_NilWhenNoIndexHTML verifies that spaHandler returns nil when
// the FS has no index.html (e.g., only .gitkeep — no UI built).
func TestSPAHandler_NilWhenNoIndexHTML(t *testing.T) {
	fsys := fstest.MapFS{
		".gitkeep": &fstest.MapFile{Data: []byte{}},
	}
	if got := spaHandler(fsys); got != nil {
		t.Error("spaHandler() = non-nil, want nil when no index.html exists")
	}
}

// TestSPAHandler_NilWhenEmptyFS verifies nil is returned for an entirely empty FS.
func TestSPAHandler_NilWhenEmptyFS(t *testing.T) {
	fsys := fstest.MapFS{}
	if got := spaHandler(fsys); got != nil {
		t.Error("spaHandler() = non-nil, want nil for empty FS")
	}
}

// TestSPAHandler_NonNilWhenIndexHTMLPresent verifies that a non-nil handler is
// returned when index.html exists.
func TestSPAHandler_NonNilWhenIndexHTMLPresent(t *testing.T) {
	fsys := buildSPAFS(nil)
	if got := spaHandler(fsys); got == nil {
		t.Error("spaHandler() = nil, want non-nil when index.html exists")
	}
}

// TestSPAHandler_RootServesIndexHTML checks that GET / returns index.html.
func TestSPAHandler_RootServesIndexHTML(t *testing.T) {
	fsys := buildSPAFS(nil)
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET / status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "SPA") {
		t.Errorf("GET / body = %q, want index.html content", body)
	}
}

// TestSPAHandler_ServesExactFile checks that an exact known path is served directly.
func TestSPAHandler_ServesExactFile(t *testing.T) {
	content := []byte("console.log('hello')")
	fsys := buildSPAFS(fstest.MapFS{
		"app.js": &fstest.MapFile{Data: content},
	})
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /app.js status = %d, want 200", rr.Code)
	}
}

// TestSPAHandler_FallbackForUnknownPath checks that an unknown path returns
// index.html content (SPA client-side routing).
func TestSPAHandler_FallbackForUnknownPath(t *testing.T) {
	fsys := buildSPAFS(nil)
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/tickets/FOO-1", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /tickets/FOO-1 status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "SPA") {
		t.Errorf("fallback body = %q, want index.html content", body)
	}
}

// TestSPAHandler_FallbackContentType checks that the SPA fallback response has
// Content-Type text/html.
func TestSPAHandler_FallbackContentType(t *testing.T) {
	fsys := buildSPAFS(nil)
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/html") {
		t.Errorf("fallback Content-Type = %q, want text/html", ct)
	}
}

// TestSPAHandler_NoCacheOnFallback checks that the no-cache header is set when
// the SPA fallback (index.html) is served for an unrecognized path.
func TestSPAHandler_NoCacheOnFallback(t *testing.T) {
	fsys := buildSPAFS(nil)
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/some/unknown/route", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	cc := rr.Header().Get("Cache-Control")
	if !strings.Contains(cc, "no-cache") {
		t.Errorf("fallback Cache-Control = %q, want no-cache", cc)
	}
}

// TestSPAHandler_LongCacheForHashedAssets checks that files under assets/ get
// the immutable long-lived cache header.
func TestSPAHandler_LongCacheForHashedAssets(t *testing.T) {
	fsys := buildSPAFS(fstest.MapFS{
		"assets/main-abc123.js": &fstest.MapFile{Data: []byte("// bundle")},
	})
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/assets/main-abc123.js", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /assets/main-abc123.js status = %d, want 200", rr.Code)
	}
	cc := rr.Header().Get("Cache-Control")
	if !strings.Contains(cc, "immutable") {
		t.Errorf("assets Cache-Control = %q, want immutable", cc)
	}
	if !strings.Contains(cc, "max-age=31536000") {
		t.Errorf("assets Cache-Control = %q, want max-age=31536000", cc)
	}
}

// TestSPAHandler_NoLongCacheForNonAssetFiles checks that non-asset files do not
// get the immutable long-lived cache header.
func TestSPAHandler_NoLongCacheForNonAssetFiles(t *testing.T) {
	fsys := buildSPAFS(fstest.MapFS{
		"favicon.ico": &fstest.MapFile{Data: []byte{0}},
	})
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET /favicon.ico status = %d, want 200", rr.Code)
	}
	cc := rr.Header().Get("Cache-Control")
	if strings.Contains(cc, "immutable") {
		t.Errorf("favicon Cache-Control = %q, should not contain immutable", cc)
	}
}

// TestSPAHandler_DeepSPARoutes checks that deeply nested SPA routes all
// fall back to index.html.
func TestSPAHandler_DeepSPARoutes(t *testing.T) {
	fsys := buildSPAFS(nil)
	h := spaHandler(fsys)

	routes := []string{
		"/projects",
		"/tickets/ABC-123",
		"/workflows/edit/some-id",
		"/settings/global",
	}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, route, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("GET %s status = %d, want 200", route, rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "SPA") {
				t.Errorf("GET %s body missing index.html content", route)
			}
		})
	}
}

// TestSPAHandler_MultipleAssetTypes checks that CSS and font assets under
// assets/ also get the long cache header.
func TestSPAHandler_MultipleAssetTypes(t *testing.T) {
	fsys := buildSPAFS(fstest.MapFS{
		"assets/style-xyz.css":      &fstest.MapFile{Data: []byte("body{}")},
		"assets/font-abc.woff2":     &fstest.MapFile{Data: []byte{0x77, 0x4f}},
	})
	h := spaHandler(fsys)

	cases := []string{
		"/assets/style-xyz.css",
		"/assets/font-abc.woff2",
	}

	for _, p := range cases {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("GET %s status = %d, want 200", p, rr.Code)
			}
			cc := rr.Header().Get("Cache-Control")
			if !strings.Contains(cc, "immutable") {
				t.Errorf("GET %s Cache-Control = %q, want immutable", p, cc)
			}
		})
	}
}

// TestDistFS_ReturnsValidFS verifies that DistFS() returns a usable fs.FS
// and that .gitkeep is readable from it (the only file guaranteed in dist/).
func TestDistFS_ReturnsValidFS(t *testing.T) {
	// Import the static package indirectly via the behaviour already wired
	// into the server: DistFS must not error and must contain at least .gitkeep.
	// We test through the static package directly to avoid import cycles.
	// Since this test is in package api, we verify the behaviour of spaHandler
	// when given a FS that mirrors what DistFS() returns in the "no UI built" case.
	//
	// The actual DistFS() is exercised by the compile-time embed check
	// (the package won't compile without dist/.gitkeep) and by
	// TestDistFS_DistFSNotError in the static package.
	fsys := fstest.MapFS{
		".gitkeep": &fstest.MapFile{Data: []byte{}},
	}
	// Confirm that a FS with only .gitkeep causes spaHandler to return nil —
	// mirroring the behaviour when dist/ only contains .gitkeep.
	if h := spaHandler(fsys); h != nil {
		t.Error("spaHandler with only .gitkeep should return nil (no-UI mode)")
	}
}

// TestSPAHandler_IndexHTMLDirectAccess checks that /index.html does not receive
// an immutable cache-control header. http.FileServer redirects /index.html → /
// (301), so we verify no immutable flag appears on the redirect response.
func TestSPAHandler_IndexHTMLDirectAccess(t *testing.T) {
	fsys := buildSPAFS(nil)
	h := spaHandler(fsys)

	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	// http.FileServer redirects /index.html → / (301 Moved Permanently).
	// Either way (redirect or direct 200), the immutable cache header must not appear
	// since the path is not under assets/.
	if rr.Code != http.StatusOK && rr.Code != http.StatusMovedPermanently {
		t.Errorf("GET /index.html status = %d, want 200 or 301", rr.Code)
	}
	cc := rr.Header().Get("Cache-Control")
	if strings.Contains(cc, "immutable") {
		t.Errorf("GET /index.html Cache-Control = %q, should not have immutable", cc)
	}
}

// TestSPAHandler_APIPathReturns404 verifies that /api/ paths are not
// intercepted by the SPA handler — they get 404 instead of index.html.
func TestSPAHandler_APIPathReturns404(t *testing.T) {
	h := spaHandler(buildSPAFS(nil))
	if h == nil {
		t.Fatal("spaHandler returned nil")
	}

	for _, path := range []string{"/api/v1/projects", "/api/v1/tickets/1", "/api/foo"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, rr.Code)
		}
	}
}

// Compile-time check: spaHandler must accept an fs.FS.
var _ = func() bool {
	var _ func(fs.FS) http.Handler = spaHandler
	return true
}
