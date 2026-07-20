package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// watcherIntTestDefaults mirrors watcherIntSettings (handlers_global_settings_watcher.go)
// so a GET on a fresh DB is verified against the same defaults the code falls
// back to when a config row is missing.
var watcherIntTestDefaults = []struct {
	key string
	def int
}{
	{"context_budget_default", 0},
	{"context_decay_turns", 20},
	{"cache_ttl_sec", 300},
	{"min_epoch_interval_calls", 20},
	{"proactive_restart_threshold_default", 250000},
	{"proactive_restart_min_interval_sec", 600},
	{"proactive_restart_max_per_session", 0},
	{"proactive_restart_boundary_window_turns", 10},
	{"proactive_restart_console_pct", 75},
}

// TestHandleGetGlobalSettings_WatcherIntDefaults verifies a fresh DB returns
// the documented int-knob defaults, present even though no config row exists
// (the <key>WithDefault fallback).
func TestHandleGetGlobalSettings_WatcherIntDefaults(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200", rr.Code)
	}
	resp := decodeSettingsResponse(t, rr)

	for _, wd := range watcherIntTestDefaults {
		v, ok := resp[wd.key]
		if !ok {
			t.Errorf("response missing key %q", wd.key)
			continue
		}
		got, isFloat := v.(float64) // JSON numbers decode as float64
		if !isFloat || int(got) != wd.def {
			t.Errorf("GET %q = %v, want %v (default)", wd.key, v, wd.def)
		}
	}
}

// TestHandleGetGlobalSettings_WatcherFloatDefault verifies the fraction knob
// defaults to 0.65 on a fresh DB.
func TestHandleGetGlobalSettings_WatcherFloatDefault(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil)
	rr := httptest.NewRecorder()
	s.handleGetGlobalSettings(rr, req)

	resp := decodeSettingsResponse(t, rr)
	v, ok := resp["context_budget_fraction"]
	if !ok {
		t.Fatalf("response missing key context_budget_fraction")
	}
	if got, isFloat := v.(float64); !isFloat || got != 0.65 {
		t.Errorf("GET context_budget_fraction = %v, want 0.65", v)
	}
}

// TestHandlePatchGlobalSettings_WatcherIntRoundTrip verifies each int knob
// can be PATCHed and read back via GET.
func TestHandlePatchGlobalSettings_WatcherIntRoundTrip(t *testing.T) {
	for _, wd := range watcherIntTestDefaults {
		wd := wd
		t.Run(wd.key, func(t *testing.T) {
			s := newGlobalSettingsServer(t)
			newVal := wd.def + 111

			body := fmt.Sprintf(`{%q:%d}`, wd.key, newVal)
			patchRR := httptest.NewRecorder()
			s.handlePatchGlobalSettings(patchRR, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(body)))
			if patchRR.Code != http.StatusOK {
				t.Fatalf("PATCH %s: status = %d, want 200; body=%s", wd.key, patchRR.Code, patchRR.Body.String())
			}

			getRR := httptest.NewRecorder()
			s.handleGetGlobalSettings(getRR, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
			resp := decodeSettingsResponse(t, getRR)
			v, ok := resp[wd.key]
			if !ok {
				t.Fatalf("GET after PATCH: missing key %q", wd.key)
			}
			if got, isFloat := v.(float64); !isFloat || int(got) != newVal {
				t.Errorf("GET after PATCH %q = %v, want %d", wd.key, v, newVal)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_WatcherFloatRoundTrip verifies
// context_budget_fraction can be PATCHed and read back.
func TestHandlePatchGlobalSettings_WatcherFloatRoundTrip(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	patchRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(patchRR, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"context_budget_fraction":0.5}`)))
	if patchRR.Code != http.StatusOK {
		t.Fatalf("PATCH status = %d, want 200; body=%s", patchRR.Code, patchRR.Body.String())
	}

	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	resp := decodeSettingsResponse(t, getRR)
	v, ok := resp["context_budget_fraction"]
	if !ok {
		t.Fatalf("GET after PATCH: missing key context_budget_fraction")
	}
	if got, isFloat := v.(float64); !isFloat || got != 0.5 {
		t.Errorf("GET after PATCH context_budget_fraction = %v, want 0.5", v)
	}
}

// TestHandlePatchGlobalSettings_WatcherFieldsPreservedOnUnrelatedPatch
// verifies a PATCH touching unrelated fields leaves watcher knobs unchanged.
func TestHandlePatchGlobalSettings_WatcherFieldsPreservedOnUnrelatedPatch(t *testing.T) {
	t.Parallel()
	s := newGlobalSettingsServer(t)

	setupRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(setupRR, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"context_decay_turns":42}`)))
	if setupRR.Code != http.StatusOK {
		t.Fatalf("setup PATCH status = %d, want 200", setupRR.Code)
	}

	unrelatedRR := httptest.NewRecorder()
	s.handlePatchGlobalSettings(unrelatedRR, httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(`{"menu_git":false}`)))
	if unrelatedRR.Code != http.StatusOK {
		t.Fatalf("unrelated PATCH status = %d, want 200", unrelatedRR.Code)
	}

	getRR := httptest.NewRecorder()
	s.handleGetGlobalSettings(getRR, httptest.NewRequest(http.MethodGet, "/api/v1/settings", nil))
	resp := decodeSettingsResponse(t, getRR)
	if v, ok := resp["context_decay_turns"]; !ok {
		t.Fatalf("GET missing key context_decay_turns")
	} else if got, isFloat := v.(float64); !isFloat || int(got) != 42 {
		t.Errorf("context_decay_turns = %v, want 42 (preserved)", v)
	}
}
