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
	cliModels, err := service.NewCLIModelService(s.deps.Pool, s.deps.Clock).ListEnabled()
	if err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("list console CLI models: %w", err)
	}
	apiModels, err := service.NewAPIModelService(s.deps.Pool, s.deps.Clock).ListEnabled()
	if err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("list console API models: %w", err)
	}
	apiMode, err := service.NewGlobalSettingsService(s.deps.Pool, s.deps.Clock).Get("api_mode_enabled")
	if err != nil {
		return types.ConsoleCatalog{}, fmt.Errorf("read API mode: %w", err)
	}

	engines := []types.ConsoleEngineOption{
		cliEngineOption("claude", "Claude", cliModels),
		cliEngineOption("codex", "Codex", cliModels),
		apiEngineOption(apiMode == "true", apiModels),
	}
	sessions, err := s.catalogSessions(projectID)
	if err != nil {
		return types.ConsoleCatalog{}, err
	}
	return types.ConsoleCatalog{ProjectID: projectID, Engines: engines, Sessions: sessions}, nil
}

func cliEngineOption(id, name string, models []*model.CLIModel) types.ConsoleEngineOption {
	result := types.ConsoleEngineOption{ID: id, DisplayName: name, Enabled: service.CLIAvailable(id)}
	if !result.Enabled {
		result.DisabledReason = id + " CLI is not installed on the server"
	}
	for _, item := range models {
		if item.CLIType == id {
			result.Models = append(result.Models, types.ConsoleModelOption{
				ID: item.ID, DisplayName: item.DisplayName, MappedModel: item.MappedModel,
				ReasoningEffort: item.ReasoningEffort,
			})
		}
	}
	return result
}

func apiEngineOption(enabled bool, models []*model.APIModel) types.ConsoleEngineOption {
	result := types.ConsoleEngineOption{
		ID: "api", DisplayName: "Direct API", Enabled: enabled, RequiresModel: true,
	}
	if !enabled {
		result.DisabledReason = "API mode is disabled"
	}
	for _, item := range models {
		result.Models = append(result.Models, types.ConsoleModelOption{
			ID: item.ID, DisplayName: item.DisplayName, Provider: item.Provider,
			MappedModel: item.MappedModel, ReasoningEffort: item.ReasoningEffort,
		})
	}
	if enabled && len(result.Models) == 0 {
		result.Enabled = false
		result.DisabledReason = "no enabled API models"
	}
	return result
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
			Status: string(row.Status), StartedAt: row.StartedAt.String,
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
