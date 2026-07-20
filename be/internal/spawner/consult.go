package spawner

import (
	"context"
	"fmt"

	"be/internal/repo"
)

// Consult synchronously spawns a named consultant agent under the caller's
// workflow instance, waits for it to finish, then reads and returns the
// _consult_answer finding. Implements apirun.ConsultantSpawner. Host callers
// with no bound workflow instance (a console session) use ConsultHost
// instead — see consult_host.go; both funnel into runConsult (consult_run.go).
func (s *Spawner) Consult(ctx context.Context, callerSessionID, consultantID, question string) (string, error) {
	pool := s.pool()
	if pool == nil {
		return "", fmt.Errorf("consult: no database pool")
	}

	sessionRepo := repo.NewAgentSessionRepo(pool, s.config.Clock)
	callerSession, err := sessionRepo.Get(callerSessionID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve caller session: %w", err)
	}

	wfiRepo := repo.NewWorkflowInstanceRepo(pool, s.config.Clock)
	wfi, err := wfiRepo.Get(callerSession.WorkflowInstanceID)
	if err != nil {
		return "", fmt.Errorf("consult: resolve workflow instance: %w", err)
	}

	def := s.loadAgentDefinition(consultantID, callerSession.ProjectID, wfi.WorkflowID)
	if def == nil {
		return "", fmt.Errorf("consult: agent definition %q not found in workflow %q", consultantID, wfi.WorkflowID)
	}

	msgRepo := repo.NewAgentMessageRepo(pool, s.config.Clock)
	messages, _ := msgRepo.GetBySession(callerSessionID)
	transcript := formatMessagesForSave(messages, maxMessageChars)

	return s.runConsult(ctx, consultRequest{
		CallerSessionID: callerSessionID,
		ProjectID:       callerSession.ProjectID,
		TicketID:        callerSession.TicketID,
		WorkflowName:    wfi.WorkflowID,
		ParentWFI:       callerSession.WorkflowInstanceID,
		ScopeType:       wfi.ScopeType,
		ConsultantID:    consultantID,
		Def:             def,
		Question:        question,
		Transcript:      transcript,
	})
}
