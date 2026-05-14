package service

import (
	"fmt"
	"slices"
	"strings"
)

const (
	ProviderClaude   = "claude"
	ProviderCodex    = "codex"
	ProviderOpencode = "opencode"
)

var (
	AllProviders = []string{ProviderClaude, ProviderCodex, ProviderOpencode}
	AllCLIModes  = []string{"cli", "cli_interactive"}
)

// ProviderSettingsService manages per-provider CLI execution-mode allowlists.
type ProviderSettingsService struct {
	gs *GlobalSettingsService
}

// NewProviderSettingsService creates a new ProviderSettingsService.
func NewProviderSettingsService(gs *GlobalSettingsService) *ProviderSettingsService {
	return &ProviderSettingsService{gs: gs}
}

// GetModes returns the allowed CLI execution modes for a provider.
// Returns [cli, cli_interactive] when no setting is stored.
func (s *ProviderSettingsService) GetModes(provider string) ([]string, error) {
	val, err := s.gs.Get("provider_" + provider + "_modes")
	if err != nil {
		return nil, err
	}
	if val == "" {
		return []string{"cli", "cli_interactive"}, nil
	}
	return strings.Split(val, ","), nil
}

// SetModes persists the allowed CLI execution modes for a provider.
// Validates provider name and mode values, dedupes, and rejects empty result.
func (s *ProviderSettingsService) SetModes(provider string, modes []string) error {
	if !slices.Contains(AllProviders, provider) {
		return fmt.Errorf("invalid provider: must be one of %s", strings.Join(AllProviders, ", "))
	}
	if len(modes) == 0 {
		return fmt.Errorf("modes must not be empty")
	}
	seen := make(map[string]bool)
	deduped := make([]string, 0, len(modes))
	for _, m := range modes {
		if !slices.Contains(AllCLIModes, m) {
			return fmt.Errorf("invalid mode %q: must be one of %s", m, strings.Join(AllCLIModes, ", "))
		}
		if !seen[m] {
			seen[m] = true
			deduped = append(deduped, m)
		}
	}
	return s.gs.Set("provider_"+provider+"_modes", strings.Join(deduped, ","))
}

// GetAll returns the allowed CLI modes for all providers.
func (s *ProviderSettingsService) GetAll() (map[string][]string, error) {
	result := make(map[string][]string, len(AllProviders))
	for _, p := range AllProviders {
		modes, err := s.GetModes(p)
		if err != nil {
			return nil, err
		}
		result[p] = modes
	}
	return result, nil
}
