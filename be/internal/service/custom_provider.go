package service

import (
	"database/sql"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/model"
	"be/internal/types"
)

var customProviderNameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

const (
	APIWireResponses       = "responses"
	APIWireChatCompletions = "chat_completions"
	APIWireOllamaNative    = "ollama_native"
)

var validAPIWires = map[string]bool{APIWireResponses: true, APIWireChatCompletions: true, APIWireOllamaNative: true}

const customProviderColumns = `name, base_url, api_key, api_wire, enabled, created_at, updated_at`

// CustomProviderService owns the registry of BYO OpenAI-compatible API
// providers (local Ollama/LM Studio/llama.cpp servers, or any other
// OpenAI-compatible endpoint).
type CustomProviderService struct {
	pool  *db.Pool
	clock clock.Clock
}

func NewCustomProviderService(pool *db.Pool, clk clock.Clock) *CustomProviderService {
	return &CustomProviderService{pool: pool, clock: clk}
}

func (s *CustomProviderService) List() ([]*model.CustomProvider, error) {
	return s.list("")
}

func (s *CustomProviderService) ListEnabled() ([]*model.CustomProvider, error) {
	return s.list("WHERE enabled = 1")
}

func (s *CustomProviderService) list(where string) ([]*model.CustomProvider, error) {
	rows, err := s.pool.Query("SELECT " + customProviderColumns + " FROM custom_providers " + where + " ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []*model.CustomProvider{}
	for rows.Next() {
		p, scanErr := scanCustomProvider(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (s *CustomProviderService) Get(name string) (*model.CustomProvider, error) {
	p, err := scanCustomProvider(s.pool.QueryRow(
		"SELECT "+customProviderColumns+" FROM custom_providers WHERE LOWER(name) = LOWER(?)", name))
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("custom provider not found: %s", name)
	}
	return p, err
}

// GetEnabled returns the provider only if it exists and is enabled.
func (s *CustomProviderService) GetEnabled(name string) (*model.CustomProvider, error) {
	p, err := s.Get(name)
	if err != nil {
		return nil, err
	}
	if !p.Enabled {
		return nil, fmt.Errorf("custom provider is disabled: %s", name)
	}
	return p, nil
}

// Exists reports whether a custom provider row exists for name, regardless of
// enabled state. Used by model.resolveProvider for existence-only lookups.
func (s *CustomProviderService) Exists(name string) (bool, error) {
	var exists int
	err := s.pool.QueryRow("SELECT 1 FROM custom_providers WHERE LOWER(name) = LOWER(?)", name).Scan(&exists)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

func validateCustomProviderName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", fmt.Errorf("name is required")
	}
	if !customProviderNameRegex.MatchString(name) {
		return "", fmt.Errorf("invalid custom provider name %q: must match ^[a-z][a-z0-9_-]*$", name)
	}
	if builtinProviders[name] {
		return "", fmt.Errorf("custom provider name %q is reserved for a built-in provider", name)
	}
	return name, nil
}

func validateBaseURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("base_url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", fmt.Errorf("invalid base_url %q: must be an http(s) URL", raw)
	}
	return raw, nil
}

func normalizeAPIWire(raw string) (string, error) {
	if raw == "" {
		return APIWireResponses, nil
	}
	if !validAPIWires[raw] {
		return "", fmt.Errorf("invalid api_wire %q: must be one of responses, chat_completions, ollama_native", raw)
	}
	return raw, nil
}

func (s *CustomProviderService) Create(req types.CustomProviderCreateRequest) (*model.CustomProvider, error) {
	name, err := validateCustomProviderName(req.Name)
	if err != nil {
		return nil, err
	}
	baseURL, err := validateBaseURL(req.BaseURL)
	if err != nil {
		return nil, err
	}
	apiWire, err := normalizeAPIWire(req.APIWire)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)
	_, err = s.pool.Exec(`INSERT INTO custom_providers (`+customProviderColumns+`)
		VALUES (?, ?, ?, ?, 1, ?, ?)`, name, baseURL, req.APIKey, apiWire, now, now)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			return nil, fmt.Errorf("custom provider already exists: %s", name)
		}
		return nil, err
	}
	return s.Get(name)
}

func scanCustomProvider(row rowScanner) (*model.CustomProvider, error) {
	var p model.CustomProvider
	var enabled int
	var createdAt, updatedAt string
	if err := row.Scan(&p.Name, &p.BaseURL, &p.APIKey, &p.APIWire, &enabled, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	p.Enabled = enabled == 1
	if t, err := time.Parse(time.RFC3339Nano, createdAt); err == nil {
		p.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339Nano, updatedAt); err == nil {
		p.UpdatedAt = t
	}
	return &p, nil
}
