package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"be/internal/service"
)

// TestHandleGetGlobalSettings_RefineryFoldStartContextPctDefault verifies a
// fresh DB reports the in-code default for the unset setting. It asserts
// against the constant rather than a literal: the value is tuned against the
// spawner's relaunch threshold, and the property under test is that the
// endpoint surfaces the default at all, not what the tuned number is.
func TestHandleGetGlobalSettings_RefineryFoldStartContextPctDefault(t *testing.T) {
	s := newGlobalSettingsServer(t)
	resp := getSettings(t, s)
	v, ok := resp["refinery_fold_start_context_pct"]
	if !ok {
		t.Fatal("response missing refinery_fold_start_context_pct field")
	}
	if int(v.(float64)) != service.DefaultRefineryFoldStartContextPct {
		t.Errorf("refinery_fold_start_context_pct = %v, want %d", v, service.DefaultRefineryFoldStartContextPct)
	}
}

// TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_SetUpdate verifies
// PATCH of an in-range value persists and is reflected by GET, including the
// boundaries 0 and 100.
func TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_SetUpdate(t *testing.T) {
	cases := []int{0, 25, 55, 100}
	for _, val := range cases {
		val := val
		t.Run(strconv.Itoa(val), func(t *testing.T) {
			s := newGlobalSettingsServer(t)
			patchSettings(t, s, `{"refinery_fold_start_context_pct":`+strconv.Itoa(val)+`}`)
			resp := getSettings(t, s)
			v := resp["refinery_fold_start_context_pct"]
			if int(v.(float64)) != val {
				t.Errorf("refinery_fold_start_context_pct = %v, want %d", v, val)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_OutOfRangeRejected
// verifies out-of-[0,100] and non-integer values 400 and leave the stored
// value untouched (asserted via a follow-up GET after a known-good PATCH).
func TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_OutOfRangeRejected(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"above_max", `{"refinery_fold_start_context_pct":101}`},
		{"negative", `{"refinery_fold_start_context_pct":-1}`},
		{"non_numeric_string", `{"refinery_fold_start_context_pct":"abc"}`},
		{"boolean", `{"refinery_fold_start_context_pct":true}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := newGlobalSettingsServer(t)
			// Establish a known value first so we can prove a rejected PATCH
			// left it untouched.
			patchSettings(t, s, `{"refinery_fold_start_context_pct":33}`)

			req := httptest.NewRequest(http.MethodPatch, "/api/v1/settings", strings.NewReader(tc.body))
			rr := httptest.NewRecorder()
			s.handlePatchGlobalSettings(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("PATCH %s: status = %d, want 400", tc.body, rr.Code)
			}

			resp := getSettings(t, s)
			v := resp["refinery_fold_start_context_pct"]
			if int(v.(float64)) != 33 {
				t.Errorf("after rejected PATCH %s, refinery_fold_start_context_pct = %v, want unchanged 33", tc.body, v)
			}
		})
	}
}

// TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_NullClears
// verifies null clears a previously set value back to the default (40).
func TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_NullClears(t *testing.T) {
	s := newGlobalSettingsServer(t)
	patchSettings(t, s, `{"refinery_fold_start_context_pct":75}`)
	patchSettings(t, s, `{"refinery_fold_start_context_pct":null}`)

	resp := getSettings(t, s)
	v := resp["refinery_fold_start_context_pct"]
	if int(v.(float64)) != service.DefaultRefineryFoldStartContextPct {
		t.Errorf("after null PATCH, refinery_fold_start_context_pct = %v, want %d (default)", v, service.DefaultRefineryFoldStartContextPct)
	}
}

// TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_AbsentPreserves
// verifies omitting the field entirely leaves a previously set value intact.
func TestHandlePatchGlobalSettings_RefineryFoldStartContextPct_AbsentPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)
	patchSettings(t, s, `{"refinery_fold_start_context_pct":62}`)

	// PATCH a different, unrelated field — must not disturb our value.
	patchSettings(t, s, `{"low_consumption_mode":true}`)

	resp := getSettings(t, s)
	v := resp["refinery_fold_start_context_pct"]
	if int(v.(float64)) != 62 {
		t.Errorf("after unrelated PATCH, refinery_fold_start_context_pct = %v, want unchanged 62", v)
	}
}
