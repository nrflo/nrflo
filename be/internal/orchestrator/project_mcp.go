package orchestrator

import (
	"context"

	"be/internal/db"
	"be/internal/logger"
	"be/internal/spawner"
)

// readExternalMCPServers loads and parses the external_mcp_servers project
// config (read once at run/spawn start, like claude_safety_hook). Invalid
// JSON is logged and ignored — the API validates at write time.
func readExternalMCPServers(ctx context.Context, pool *db.Pool, projectID string) map[string]spawner.ExternalMCPServer {
	raw, _ := pool.GetProjectConfig(projectID, "external_mcp_servers")
	if raw == "" {
		return nil
	}
	servers, err := spawner.ParseExternalMCPServers(raw)
	if err != nil {
		logger.Warn(ctx, "invalid external_mcp_servers config ignored", "project", projectID, "error", err)
		return nil
	}
	return servers
}
