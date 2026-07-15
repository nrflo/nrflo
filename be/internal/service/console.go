package service

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/clock"
	"be/internal/db"
	"be/internal/id"
	"be/internal/model"
	"be/internal/repo"
)

// ErrConsoleSessionNotFound is returned when a console session id does not
// resolve to a kind='console' agent_sessions row.
var ErrConsoleSessionNotFound = errors.New("console_session_not_found")

// ErrConsoleProjectNotFound is returned by CreateSession when the project does
// not exist.
var ErrConsoleProjectNotFound = errors.New("console_project_not_found")

// ConsoleIdleTTLHoursKey is the global config-KV key for console idle expiry.
const ConsoleIdleTTLHoursKey = "console_idle_ttl_hours"

// DefaultConsoleIdleTTLHours is used when ConsoleIdleTTLHoursKey is unset.
const DefaultConsoleIdleTTLHours = 12

// ConsoleService manages console session lifecycle: sessions with no
// workflow_instance_id, authenticated via the same spawn-token bearer path
// as spawned agents.
type ConsoleService struct {
	pool  *db.Pool
	clock clock.Clock
}

// NewConsoleService creates a new ConsoleService.
func NewConsoleService(pool *db.Pool, clk clock.Clock) *ConsoleService {
	return &ConsoleService{pool: pool, clock: clk}
}

// CreateSession validates the project exists and inserts a kind='console'
// agent_sessions row with a freshly minted bearer token. Returns the session
// id and the token (exposed to the caller exactly once). ticketID is stored
// verbatim as the session's "current ticket" (read back by the ticket_current
// tool); callers validate it against the project first and pass "" when the
// caller has no ticket context — CreateSession trusts it and does not re-check.
func (s *ConsoleService) CreateSession(projectID, ticketID string) (sessionID, token string, err error) {
	projectRepo := repo.NewProjectRepo(s.pool, s.clock)
	exists, err := projectRepo.Exists(projectID)
	if err != nil {
		return "", "", fmt.Errorf("check project: %w", err)
	}
	if !exists {
		return "", "", ErrConsoleProjectNotFound
	}

	sessionID = uuid.New().String()
	token = id.MintToken()
	now := s.clock.Now().UTC().Format(time.RFC3339Nano)

	sessionRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	sess := &model.AgentSession{
		ID:         sessionID,
		ProjectID:  projectID,
		TicketID:   ticketID,
		Phase:      "console",
		NodeID:     "console",
		AgentType:  "console",
		Status:     model.AgentSessionUserInteractive,
		Kind:       model.AgentSessionKindConsole,
		SpawnToken: sql.NullString{String: token, Valid: true},
		StartedAt:  sql.NullString{String: now, Valid: true},
	}
	if err := sessionRepo.Create(sess); err != nil {
		return "", "", fmt.Errorf("create console session: %w", err)
	}

	return sessionID, token, nil
}

// CloseSession marks a console session interactive_completed, which kills its
// bearer token via the GetByToken status filter. A no-op (nil error) if the
// session is already closed; returns ErrConsoleSessionNotFound if id does not
// resolve to a console row.
func (s *ConsoleService) CloseSession(sessionID string) error {
	sessionRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	sess, err := sessionRepo.GetConsole(sessionID)
	if err != nil {
		return fmt.Errorf("get console session: %w", err)
	}
	if sess == nil {
		return ErrConsoleSessionNotFound
	}
	if _, err := sessionRepo.CloseConsole(sessionID); err != nil {
		return fmt.Errorf("close console session: %w", err)
	}
	return nil
}

// SweepIdle expires console sessions whose updated_at is older than the
// global console_idle_ttl_hours config (default DefaultConsoleIdleTTLHours).
// Mirrors PlanService.SweepExpiredDrafts. Returns the number expired.
func (s *ConsoleService) SweepIdle(now time.Time) (int64, error) {
	ttlHours := SubworkflowCap(s.pool, "", ConsoleIdleTTLHoursKey, DefaultConsoleIdleTTLHours)
	cutoff := now.Add(-time.Duration(ttlHours) * time.Hour).UTC().Format(time.RFC3339Nano)
	sessionRepo := repo.NewAgentSessionRepo(s.pool, s.clock)
	return sessionRepo.ExpireIdleConsoles(cutoff)
}
