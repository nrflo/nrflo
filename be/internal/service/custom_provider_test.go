package service

import (
	"path/filepath"
	"strings"
	"testing"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/types"
)

func setupCustomProviderService(t *testing.T) *CustomProviderService {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "custom_providers.db")
	if err := svcCopyTemplateDB(dbPath); err != nil {
		t.Fatalf("copy template DB: %v", err)
	}
	pool, err := db.OpenPoolExisting(dbPath, db.DefaultPoolConfig())
	if err != nil {
		t.Fatalf("open pool: %v", err)
	}
	t.Cleanup(func() { pool.Close() })
	return NewCustomProviderService(pool, clock.Real())
}

func validCreateReq() types.CustomProviderCreateRequest {
	return types.CustomProviderCreateRequest{
		Name:    "local-ollama",
		BaseURL: "http://localhost:11434/v1",
	}
}

// TestCustomProviderCreate_HappyPath verifies a minimal create (no api_key)
// defaults api_wire to "responses" and enabled=1.
func TestCustomProviderCreate_HappyPath(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	p, err := svc.Create(validCreateReq())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Name != "local-ollama" || p.BaseURL != "http://localhost:11434/v1" {
		t.Errorf("unexpected provider: %+v", p)
	}
	if p.APIWire != APIWireResponses {
		t.Errorf("APIWire = %q, want %q (default)", p.APIWire, APIWireResponses)
	}
	if !p.Enabled {
		t.Error("Enabled = false, want true on create")
	}
	if p.APIKey != "" {
		t.Errorf("APIKey = %q, want empty (not supplied)", p.APIKey)
	}
}

// TestCustomProviderCreate_NameValidation table-drives the name regex +
// reserved-name rejection rules.
//
// NOTE (production bug, see be_production_bugs finding): validateCustomProviderName
// checks `if builtinProviders[name]`, i.e. the *value* (apiOnly) rather than
// map presence. Since anthropic/openai have apiOnly=false, that check is a
// no-op for them — only openrouter (apiOnly=true) actually gets rejected as
// reserved. "anthropic"/"openai" are documented here as currently ACCEPTED,
// which contradicts the ticket's "rejects builtin-reserved names
// (anthropic/openai/openrouter)" requirement.
func TestCustomProviderCreate_NameValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		reqName string
		wantErr string
	}{
		{"empty", "", "name is required"},
		{"reserved openrouter", "openrouter", "reserved for a built-in provider"},
		{"leading digit rejected", "1local", "invalid custom provider name"},
		{"leading dash rejected", "-local", "invalid custom provider name"},
		{"space rejected", "local ollama", "invalid custom provider name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := setupCustomProviderService(t)
			req := validCreateReq()
			req.Name = tc.reqName
			_, err := svc.Create(req)
			if err == nil {
				t.Fatalf("Create(name=%q) succeeded, want error", tc.reqName)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want mention of %q", err, tc.wantErr)
			}
		})
	}
}

// TestCustomProviderCreate_NameNormalizedToLowercase verifies a mixed-case
// name is accepted and stored lowercased (the regex check runs after
// lowercasing, so this is not itself a rejection case).
func TestCustomProviderCreate_NameNormalizedToLowercase(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	req := validCreateReq()
	req.Name = "Local-Ollama"
	p, err := svc.Create(req)
	if err != nil {
		t.Fatalf("Create(mixed-case name): %v", err)
	}
	if p.Name != "local-ollama" {
		t.Errorf("Name = %q, want lowercased local-ollama", p.Name)
	}
}

// TestCustomProviderCreate_AnthropicOpenAINames_NotRejected pins the current
// (buggy) behavior of the reserved-name check: see the production-bug note
// on TestCustomProviderCreate_NameValidation. Once be_production_bugs is
// fixed to check map presence, this test should be inverted to expect
// rejection for both names.
func TestCustomProviderCreate_AnthropicOpenAINames_NotRejected(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"anthropic", "openai"} {
		t.Run(name, func(t *testing.T) {
			svc := setupCustomProviderService(t)
			req := validCreateReq()
			req.Name = name
			if _, err := svc.Create(req); err != nil {
				t.Fatalf("Create(name=%q) = %v; current (buggy) behavior expects success — see production-bug note", name, err)
			}
		})
	}
}

// TestCustomProviderCreate_NameValidAcceptedChars verifies lowercase letters,
// digits, underscore, and dash are all accepted after the leading letter.
func TestCustomProviderCreate_NameValidAcceptedChars(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	req := validCreateReq()
	req.Name = "local_ollama-2"
	p, err := svc.Create(req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.Name != "local_ollama-2" {
		t.Errorf("Name = %q, want local_ollama-2", p.Name)
	}
}

// TestCustomProviderCreate_DuplicateName_Rejected verifies a second Create
// with the same name (case-insensitively) is rejected as already-exists.
func TestCustomProviderCreate_DuplicateName_Rejected(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	if _, err := svc.Create(validCreateReq()); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	req := validCreateReq()
	req.Name = "LOCAL-OLLAMA"
	_, err := svc.Create(req)
	if err == nil {
		t.Fatal("second Create(dup name) succeeded, want error")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("error = %v, want mention of already exists", err)
	}
}

// TestCustomProviderCreate_BaseURLValidation table-drives the base_url
// required + http(s)-URL rules.
func TestCustomProviderCreate_BaseURLValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		baseURL string
		wantErr string
	}{
		{"empty", "", "base_url is required"},
		{"no scheme", "localhost:11434", "invalid base_url"},
		{"ftp scheme", "ftp://localhost:11434", "invalid base_url"},
		{"scheme no host", "http://", "invalid base_url"},
		{"garbage", "not a url at all", "invalid base_url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			svc := setupCustomProviderService(t)
			req := validCreateReq()
			req.BaseURL = tc.baseURL
			_, err := svc.Create(req)
			if err == nil {
				t.Fatalf("Create(base_url=%q) succeeded, want error", tc.baseURL)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %v, want mention of %q", err, tc.wantErr)
			}
		})
	}
}

// TestCustomProviderCreate_APIWire_DefaultAndEnum verifies the default value
// plus enum rejection.
func TestCustomProviderCreate_APIWire_DefaultAndEnum(t *testing.T) {
	t.Parallel()

	t.Run("explicit chat_completions accepted", func(t *testing.T) {
		svc := setupCustomProviderService(t)
		req := validCreateReq()
		req.APIWire = APIWireChatCompletions
		p, err := svc.Create(req)
		if err != nil {
			t.Fatalf("Create: %v", err)
		}
		if p.APIWire != APIWireChatCompletions {
			t.Errorf("APIWire = %q, want %q", p.APIWire, APIWireChatCompletions)
		}
	})

	t.Run("invalid enum rejected", func(t *testing.T) {
		svc := setupCustomProviderService(t)
		req := validCreateReq()
		req.APIWire = "streaming"
		_, err := svc.Create(req)
		if err == nil {
			t.Fatal("Create(invalid api_wire) succeeded, want error")
		}
		if !strings.Contains(err.Error(), "invalid api_wire") {
			t.Errorf("error = %v, want mention of invalid api_wire", err)
		}
	})
}

// TestCustomProviderCreate_APIKeyOptional verifies an explicit api_key round-trips.
func TestCustomProviderCreate_APIKeyOptional(t *testing.T) {
	t.Parallel()
	svc := setupCustomProviderService(t)
	req := validCreateReq()
	req.APIKey = "sk-local-test"
	p, err := svc.Create(req)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.APIKey != "sk-local-test" {
		t.Errorf("APIKey = %q, want sk-local-test", p.APIKey)
	}
}
