package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var menuTestDefaults = []struct {
	key string
	def bool
}{
	{"menu_new_ticket", false},
	{"menu_import_spec", false},
	{"menu_git", true},
	{"menu_chain_executions", true},
	{"menu_schedules", false},
	{"menu_workflow_chains", false},
	{"menu_python_scripts", false},
	{"menu_documentation", true},
	{"menu_errors", false},
	{"menu_agent_sessions", false},
}

// TestHandleGetGlobalSettings_MenuPanelDefaults verifies fresh DB returns correct defaults for all 10 menu keys.
func TestHandleGetGlobalSettings_MenuPanelDefaults(t *testing.T) {
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}

	resp := decodeSettingsResponse(t, rr)

	for _, md := range menuTestDefaults {
		v, ok := resp[md.key]
		if !ok {
			t.Errorf("response missing key %q", md.key)
			continue
		}
		if v != md.def {
			t.Errorf("GET %q = %v, want %v (default)", md.key, v, md.def)
		}
	}
}

// TestHandlePatchGlobalSettings_MenuPanelRoundTrip verifies each menu key can be toggled away from its
// default and restored, with GET reflecting the correct value at each step.
func TestHandlePatchGlobalSettings_MenuPanelRoundTrip(t *testing.T) {
	for _, md := range menuTestDefaults {
		md := md
		t.Run(md.key, func(t *testing.T) {
			s := newGlobalSettingsServer(t)
			toggled := !md.def

			// PATCH to toggled value
			body1 := fmt.Sprintf(`{%q:%v}`, md.key, toggled)
			rr1 := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr1, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body1)))
			if rr1.Code != http.StatusOK {
				t.Fatalf("PATCH toggle %s: status = %d, want 200", md.key, rr1.Code)
			}

			// GET should reflect toggled value
			rr2 := httptest.NewRecorder()
			s.handleGetGlobalSettings(rr2, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
			resp1 := decodeSettingsResponse(t, rr2)
			if v, ok := resp1[md.key]; !ok {
				t.Errorf("GET after toggle: missing key %q", md.key)
			} else if v != toggled {
				t.Errorf("GET after toggle: %q = %v, want %v", md.key, v, toggled)
			}

			// PATCH back to default
			body2 := fmt.Sprintf(`{%q:%v}`, md.key, md.def)
			rr3 := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr3, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body2)))
			if rr3.Code != http.StatusOK {
				t.Fatalf("PATCH restore %s: status = %d, want 200", md.key, rr3.Code)
			}

			// GET should reflect restored default
			rr4 := httptest.NewRecorder()
			s.handleGetGlobalSettings(rr4, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
			resp2 := decodeSettingsResponse(t, rr4)
			if v, ok := resp2[md.key]; !ok {
				t.Errorf("GET after restore: missing key %q", md.key)
			} else if v != md.def {
				t.Errorf("GET after restore: %q = %v, want %v", md.key, v, md.def)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_MenuPanelEmptyBodyPreserves verifies an empty PATCH body leaves all
// previously set menu values unchanged (mirrors TestHandlePatchGlobalSettings_NullFieldPreserves).
func TestHandlePatchGlobalSettings_MenuPanelEmptyBodyPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)

	// Enable two keys that default to false, and record that default-true keys remain true
	req1 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings",
		strings.NewReader(`{"menu_new_ticket":true,"menu_schedules":true}`))
	rr1 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr1, req1)
	if rr1.Code != http.StatusOK {
		t.Fatalf("setup PATCH status = %d, want 200", rr1.Code)
	}

	// PATCH with empty body — must not change any value
	req2 := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{}`))
	rr2 := httptest.NewRecorder()
	s.handlePatchGlobalSettings(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("empty PATCH status = %d, want 200", rr2.Code)
	}

	// GET and verify
	rr3 := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr3, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	if rr3.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr3.Code)
	}

	resp := decodeSettingsResponse(t, rr3)

	want := map[string]bool{
		"menu_new_ticket":       true,  // explicitly enabled above
		"menu_schedules":        true,  // explicitly enabled above
		"menu_git":              true,  // default true, untouched
		"menu_chain_executions": true,  // default true, untouched
		"menu_documentation":    true,  // default true, untouched
		"menu_import_spec":      false, // default false, untouched
		"menu_workflow_chains":  false, // default false, untouched
		"menu_python_scripts":   false, // default false, untouched
		"menu_errors":           false, // default false, untouched
		"menu_agent_sessions":   false, // default false, untouched
	}

	for key, wantVal := range want {
		v, ok := resp[key]
		if !ok {
			t.Errorf("response missing key %q", key)
			continue
		}
		if v != wantVal {
			t.Errorf("after empty PATCH: GET %q = %v, want %v", key, v, wantVal)
		}
	}
}
