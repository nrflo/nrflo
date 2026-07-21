package console

import (
	"errors"
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/types"
)

// ErrChatProjectMismatch prevents the trusted socket from returning a live
// chat bearer to a caller resolved into another project.
var ErrChatProjectMismatch = errors.New("console_chat_project_mismatch")

// Catalog returns the live resumable chats and enabled model registry entries
// for the native console. Engine availability is decided by the server.
func (s *ChatService) Catalog(projectID string) (types.ConsoleCatalog, error) {
	if projectID == "" {
		return types.ConsoleCatalog{}, service.ErrConsoleProjectNotFound
	}
	exists, err := repo.NewProjectRepo(s.deps.Pool, s.deps.Clock).Exists(projectID)
	if err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("check console project: %w", err)
	}
	if !exists {
		return types.ConsoleCatalog{}, service.ErrConsoleProjectNotFound
	}
	models, err := service.NewModelService(s.deps.Pool, s.deps.Clock).ListEnabled()
	if err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("list console models: %w", err)
	}
	apiMode, err := service.NewGlobalSettingsService(s.deps.Pool, s.deps.Clock).Get("api_mode_enabled")
	if err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("read API mode: %w", err)
	}

	engines := []types.ConsoleEngineOption{
		cliEngineOption("claude", "Claude", models),
		cliEngineOption("codex", "Codex", models),
		apiEngineOption(apiMode == "true", models),
	}
	sessions, err := s.catalogSessions(projectID)
	if err != nil {
		return types.ConsoleCatalog{}, err
	}
	return types.ConsoleCatalog{ProjectID: projectID, Engines: engines, Sessions: sessions, Profiles: catalogProfiles()}, nil
}

// catalogProfiles maps the built-in console.Profile registry onto the
// catalog's wire shape.
func catalogProfiles() []types.ConsoleProfileOption {
	profiles := ListProfiles()
	out := make([]types.ConsoleProfileOption, 0, len(profiles))
	for _, p := range profiles {
		out = append(out, types.ConsoleProfileOption{
			Name:                p.Name,
			DisplayName:         p.DisplayName,
			Description:         p.Description,
			DefaultEngine:       p.DefaultEngine,
			DefaultModelID:      p.DefaultModelID,
			DefaultEffort:       p.DefaultEffort,
			ContextBudgetTokens: p.ContextBudgetTokens,
			RefineryDefault:     p.RefineryDefault,
			SystemTemplateID:    p.SystemTemplateID,
		})
	}
	return out
}

// brandOf maps a cli_type or api provider onto the model-family grouping key
// pickers drill down by: claude/anthropic → "claude", codex/openai → "gpt".
func brandOf(provider string) string {
	switch provider {
	case "claude", "anthropic":
		return "claude"
	case "codex", "openai":
		return "gpt"
	}
	return provider
}

func cliEngineOption(id, name string, models []*model.Model) types.ConsoleEngineOption {
	result := types.ConsoleEngineOption{
		ID: id, DisplayName: name, Kind: "cli", Brand: brandOf(id), Enabled: service.CLIAvailable(id),
	}
	if !result.Enabled {
		result.DisabledReason = id + " CLI is not installed on the server"
	}
	own := make([]*model.Model, 0, len(models))
	for _, item := range models {
		if item.CLIModel != "" && cliEngineForProvider(item.Provider) == id {
			own = append(own, item)
		}
	}
	service.SortModelsForPicker(own)
	for _, item := range own {
		result.Models = append(result.Models, types.ConsoleModelOption{
			ID: item.ID, DisplayName: item.DisplayName, Brand: result.Brand,
			Provider: item.Provider, MappedModel: item.CLIModel, ReasoningEffort: item.DefaultEffort,
			SupportedEfforts: item.CLIEfforts,
		})
	}
	return result
}

func apiEngineOption(enabled bool, models []*model.Model) types.ConsoleEngineOption {
	result := types.ConsoleEngineOption{
		ID: "api", DisplayName: "Direct API", Kind: "api", Enabled: enabled, RequiresModel: true,
	}
	if !enabled {
		result.DisabledReason = "API mode is disabled"
	}
	sorted := make([]*model.Model, 0, len(models))
	for _, item := range models {
		if item.APIModel != "" {
			sorted = append(sorted, item)
		}
	}
	service.SortModelsForPicker(sorted)
	for _, item := range sorted {
		result.Models = append(result.Models, types.ConsoleModelOption{
			ID: item.ID, DisplayName: item.DisplayName, Brand: brandOf(item.Provider),
			Provider: item.Provider, MappedModel: item.APIModel, ReasoningEffort: item.DefaultEffort,
			SupportedEfforts: item.APIEfforts,
		})
	}
	if enabled && len(result.Models) == 0 {
		result.Enabled = false
		result.DisabledReason = "no enabled API models"
	}
	return result
}

func cliEngineForProvider(provider string) string {
	switch provider {
	case "anthropic":
		return "claude"
	case "openai":
		return "codex"
	default:
		return ""
	}
}

func (s *ChatService) catalogSessions(projectID string) ([]types.ConsoleSessionOption, error) {
	rows, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).ListConsoleChats(projectID, 20)
	if err != nil {
		return nil, fmt.Errorf("list console chats: %w", err)
	}
	result := make([]types.ConsoleSessionOption, 0, len(rows))
	for _, row := range rows {
		if !s.Live(row.ID) {
			continue
		}
		item := types.ConsoleSessionOption{
			SessionID: row.ID, Engine: row.ConsoleEngine.String, Model: row.ModelID.String,
			Status: string(row.Status), StartedAt: row.StartedAt.String, Profile: row.ConsoleProfile,
		}
		if row.ContextLeft.Valid {
			value := int(row.ContextLeft.Int64)
			item.ContextLeft = &value
		}
		result = append(result, item)
	}
	return result, nil
}

// AttachAuthenticated returns an existing live chat's bearer to a trusted
// local socket caller. The bearer is shared with the provider MCP bridge and
// therefore must not be rotated.
func (s *ChatService) AttachAuthenticated(sessionID, projectID string) (string, error) {
	sess, ok := s.get(sessionID)
	if !ok {
		return "", ErrChatSessionNotFound
	}
	if !strings.EqualFold(sess.ProjectID(), projectID) {
		return "", ErrChatProjectMismatch
	}
	row, err := repo.NewAgentSessionRepo(s.deps.Pool, s.deps.Clock).GetConsoleChat(sessionID)
	if err != nil {
		return "", fmt.Errorf("load console chat: %w", err)
	}
	if row == nil || !row.SpawnToken.Valid || row.SpawnToken.String == "" {
		return "", ErrChatSessionNotFound
	}
	return row.SpawnToken.String, nil
}
