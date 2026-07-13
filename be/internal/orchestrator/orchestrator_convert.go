package orchestrator

import (
	"context"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/logger"
	"be/internal/repo"
	"be/internal/service"
	"be/internal/spawner"
	"be/internal/ws"
)

// convertToSpawnerWorkflows converts service types to spawner types.
func convertToSpawnerWorkflows(svc map[string]service.SpawnerWorkflowDef) map[string]spawner.WorkflowDef {
	result := make(map[string]spawner.WorkflowDef, len(svc))
	for name, swf := range svc {
		var phases []spawner.PhaseDef
		for _, sp := range swf.Phases {
			phases = append(phases, spawner.PhaseDef{
				NodeID:       sp.NodeID,
				Agent:        sp.Agent,
				Layer:        sp.Layer,
				Instructions: sp.Instructions,
			})
		}
		result[name] = spawner.WorkflowDef{
			Description: swf.Description,
			ScopeType:   swf.ScopeType,
			Phases:      phases,
		}
	}
	return result
}

// convertToSpawnerAgents converts service types to spawner types.
func convertToSpawnerAgents(svc map[string]service.SpawnerAgentConfig) map[string]spawner.AgentConfig {
	result := make(map[string]spawner.AgentConfig, len(svc))
	for name, sa := range svc {
		result[name] = spawner.AgentConfig{
			Model:   sa.Model,
			Timeout: sa.Timeout,
		}
	}
	return result
}

// RecordUserInput persists a user-typed line for the given session.
// Delegates to the active spawner if the session belongs to a live proc;
// falls back to a direct DB insert when no active spawner owns the session
// (user_interactive / resume-session cases).
func (o *Orchestrator) RecordUserInput(sessionID, text string) {
	o.mu.Lock()
	seen := make(map[*spawner.Spawner]struct{})
	var uniqueSpawners []*spawner.Spawner
	for _, rs := range o.runs {
		for _, sp := range rs.spawners {
			if _, ok := seen[sp]; ok {
				continue
			}
			seen[sp] = struct{}{}
			uniqueSpawners = append(uniqueSpawners, sp)
		}
	}
	o.mu.Unlock()

	for _, sp := range uniqueSpawners {
		if sp.RecordUserInput(sessionID, text) {
			return
		}
	}

	// No active spawner owns this session — insert directly into the DB.
	recordUserInputFallback(o.dataPath, o.clock, o.wsHub, sessionID, text)
}

// recordUserInputFallback inserts a user_input message row and broadcasts
// EventMessagesUpdated. Used when no live spawner proc is tracking the session
// (e.g. user_interactive take-control or resume-session flows).
func recordUserInputFallback(dataPath string, clk clock.Clock, hub *ws.Hub, sessionID, text string) {
	database, err := db.Open(dataPath)
	if err != nil {
		return
	}
	defer database.Close()

	pool := db.WrapAsPool(database)
	msgRepo := repo.NewAgentMessageRepo(pool, clk)
	if err := msgRepo.InsertBatch(sessionID, []repo.MessageEntry{{Content: text, Category: "user_input"}}); err != nil {
		logger.Warn(context.Background(), "user input fallback: insert failed", "session_id", sessionID, "err", err)
		return
	}

	if hub == nil {
		return
	}

	asRepo := repo.NewAgentSessionRepo(database, clk)
	session, err := asRepo.Get(sessionID)
	if err != nil {
		return
	}
	wfiRepo := repo.NewWorkflowInstanceRepo(pool, clk)
	wfi, err := wfiRepo.Get(session.WorkflowInstanceID)
	if err != nil {
		return
	}

	modelID := ""
	if session.ModelID.Valid {
		modelID = session.ModelID.String
	}
	hub.Broadcast(ws.NewEvent(ws.EventMessagesUpdated, session.ProjectID, session.TicketID, wfi.WorkflowID, map[string]interface{}{
		"session_id": sessionID,
		"agent_type": session.AgentType,
		"model_id":   modelID,
	}))
}
