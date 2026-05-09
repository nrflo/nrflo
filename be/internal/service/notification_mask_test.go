package service

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestMaskConfig_Slack_MasksLastFour(t *testing.T) {
	t.Parallel()
	config := `{"webhook_url":"https://hooks.slack.com/services/ABC/DEF/GHIJ"}`
	result := maskConfig("slack", config)
	if strings.Contains(result, "GHIJ") {
		t.Errorf("maskConfig slack: last 4 chars not masked, got %q", result)
	}
	if !strings.Contains(result, "****") {
		t.Errorf("maskConfig slack: no **** in result: %q", result)
	}
}

func TestMaskConfig_Telegram_MasksToken_PassesChatID(t *testing.T) {
	t.Parallel()
	config := `{"bot_token":"1234567890:ABCDEFGH","chat_id":"-100123"}`
	result := maskConfig("telegram", config)
	if strings.Contains(result, "ABCDEFGH") {
		t.Errorf("bot_token not masked in: %q", result)
	}
	if !strings.Contains(result, "-100123") {
		t.Errorf("chat_id not preserved in: %q", result)
	}
}

func TestMaskConfig_InvalidJSON_Passthrough(t *testing.T) {
	t.Parallel()
	bad := `not-json`
	if result := maskConfig("slack", bad); result != bad {
		t.Errorf("maskConfig invalid JSON: got %q, want passthrough %q", result, bad)
	}
}

func TestMaskToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"1234ABCDEFGH5678", "1234****5678"},
		{"short", "****"},
		{"12345678", "****"},
		{"123456789", "1234****6789"},
	}
	for _, tc := range tests {
		if got := maskToken(tc.input); got != tc.want {
			t.Errorf("maskToken(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestMaskURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"https://hooks.slack.com/GHIJ", "https://hooks.slack.com/****"},
		{"abcd", "****"},
		{"abc", "****"},
	}
	for _, tc := range tests {
		if got := maskURL(tc.input); got != tc.want {
			t.Errorf("maskURL(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestApplyConfigPatch_MaskedValuePreservesSecret(t *testing.T) {
	t.Parallel()
	stored := `{"webhook_url":"https://hooks.slack.com/services/ABC/XYZW"}`
	masked := maskConfig("slack", stored)

	var maskedMap map[string]interface{}
	if err := json.Unmarshal([]byte(masked), &maskedMap); err != nil {
		t.Fatalf("unmarshal masked: %v", err)
	}
	maskedURL, _ := maskedMap["webhook_url"].(string)
	incoming, _ := json.Marshal(map[string]interface{}{"webhook_url": maskedURL})
	result := applyConfigPatch("slack", stored, string(incoming))

	if !strings.Contains(result, "XYZW") {
		t.Errorf("applyConfigPatch: did not preserve secret; got %q", result)
	}
}

func TestApplyConfigPatch_NewValueRotatesSecret(t *testing.T) {
	t.Parallel()
	stored := `{"webhook_url":"https://hooks.slack.com/old-secret-XXXX"}`
	newURL := "https://hooks.slack.com/new-url-YYYY"
	incoming, _ := json.Marshal(map[string]interface{}{"webhook_url": newURL})
	result := applyConfigPatch("slack", stored, string(incoming))

	if !strings.Contains(result, "YYYY") {
		t.Errorf("applyConfigPatch: new value not stored; got %q", result)
	}
	if strings.Contains(result, "XXXX") {
		t.Errorf("applyConfigPatch: old value still present; got %q", result)
	}
}
