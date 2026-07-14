package api

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"be/internal/model"
	"be/internal/repo"

	"github.com/google/uuid"
)

// appendConsoleToolAudit records one console tool call as an audit_log row
// keyed to the console session (resource_type=agent_session,
// resource_id=<session id>), so GET /api/v1/audit-log?resource_type=agent_session&resource_id=<id>
// returns that session's tool-call trail. Reuses repo.AuditRepo.Append, which
// already writes user_id NULL for bearer principals (audit_repo.go:60).
//
// projectID is the project the call actually acted on (consoleToolProject), not
// sess.ProjectID: a global-scope console session may target another project via
// X-Project, and the audit trail must name that project, not `__global__`.
func appendConsoleToolAudit(s *Server, r *http.Request, sess *model.AgentSession, projectID, toolName string, args json.RawMessage, dur time.Duration, outcome string) {
	sum := sha256.Sum256(args)
	digest := hex.EncodeToString(sum[:])[:16]

	metadata, _ := json.Marshal(map[string]interface{}{
		"tool":        toolName,
		"args_digest": digest,
		"duration_ms": dur.Milliseconds(),
		"outcome":     outcome,
		"project":     projectID,
	})

	_ = repo.NewAuditRepo(s.pool, s.clock).Append(&model.AuditEntry{
		ID:           uuid.New().String(),
		Action:       "console.tool.call",
		ResourceType: "agent_session",
		ResourceID:   sess.ID,
		IP:           r.RemoteAddr,
		UserAgent:    r.UserAgent(),
		Metadata:     string(metadata),
	})
}
