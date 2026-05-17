package notify

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"

	"be/internal/model"
)

// StartNotifySession inserts a transient _notification agent_session for SDK auth.
// Returns ("", "", nil) when instanceID is empty — transport runs without SDK auth.
func StartNotifySession(projectID, instanceID string) (sessionID, spawnToken string, err error) {
	if instanceID == "" {
		return "", "", nil
	}
	rt := getScriptRuntime()
	if rt == nil || rt.SessionRepo == nil {
		return "", "", nil
	}

	token := mintNotifyToken()
	sid := uuid.New().String()
	now := rt.Clock.Now().UTC().Format(time.RFC3339Nano)

	sess := &model.AgentSession{
		ID:                 sid,
		ProjectID:          projectID,
		WorkflowInstanceID: instanceID,
		AgentType:          "_notification",
		Status:             model.AgentSessionRunning,
		SpawnToken:         sql.NullString{String: token, Valid: true},
		StartedAt:          sql.NullString{String: now, Valid: true},
	}
	if err := rt.SessionRepo.Create(sess); err != nil {
		return "", "", fmt.Errorf("notify: start session: %w", err)
	}
	return sid, token, nil
}

// EndNotifySession marks the session as completed or failed. Noop on empty sessionID.
func EndNotifySession(sessionID string, success bool) {
	if sessionID == "" {
		return
	}
	rt := getScriptRuntime()
	if rt == nil || rt.SessionRepo == nil {
		return
	}
	if success {
		_ = rt.SessionRepo.UpdateStatusEnded(sessionID, model.AgentSessionCompleted)
	} else {
		_ = rt.SessionRepo.UpdateStatusToFailedWithReason(sessionID, "notification script failed")
	}
}

func mintNotifyToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return uuid.New().String() + uuid.New().String()
	}
	return hex.EncodeToString(buf)
}
