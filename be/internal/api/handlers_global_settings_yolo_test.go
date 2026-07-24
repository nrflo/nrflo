package api

import "testing"

// TestGlobalSettings_ConsoleYolo_DefaultOn verifies a fresh DB reports
// console_yolo=true (default-ON, unlike the other bool fields in
// globalSettingsBoolFields which default to false).
func TestGlobalSettings_ConsoleYolo_DefaultOn(t *testing.T) {
	s := newGlobalSettingsServer(t)
	resp := getSettings(t, s)
	v, ok := resp["console_yolo"]
	if !ok {
		t.Fatal("response missing console_yolo field")
	}
	if v != true {
		t.Errorf("console_yolo = %v, want true (default-ON)", v)
	}
}

// TestGlobalSettings_ConsoleYolo_PatchRoundTrip verifies PATCH
// console_yolo=false is reflected on GET, and PATCH true flips it back.
func TestGlobalSettings_ConsoleYolo_PatchRoundTrip(t *testing.T) {
	s := newGlobalSettingsServer(t)

	patchSettings(t, s, `{"console_yolo":false}`)
	resp := getSettings(t, s)
	if resp["console_yolo"] != false {
		t.Errorf("after PATCH false, console_yolo = %v, want false", resp["console_yolo"])
	}

	patchSettings(t, s, `{"console_yolo":true}`)
	resp = getSettings(t, s)
	if resp["console_yolo"] != true {
		t.Errorf("after PATCH true, console_yolo = %v, want true", resp["console_yolo"])
	}
}

// TestGlobalSettings_ConsoleYolo_AbsentPreserves verifies a PATCH that omits
// console_yolo leaves a previously-set value unchanged.
func TestGlobalSettings_ConsoleYolo_AbsentPreserves(t *testing.T) {
	s := newGlobalSettingsServer(t)

	patchSettings(t, s, `{"console_yolo":false}`)
	patchSettings(t, s, `{"low_consumption_mode":true}`)

	resp := getSettings(t, s)
	if resp["console_yolo"] != false {
		t.Errorf("console_yolo = %v, want false (preserved across unrelated PATCH)", resp["console_yolo"])
	}
}
