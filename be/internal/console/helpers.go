package console

import (
	"fmt"
	"strings"

	"be/internal/model"
	"be/internal/repo"
)

// invalidArgs and missingService mirror tools_builtin's convention: user /
// argument errors return (msg, true, nil) — never a Go error, which is
// reserved for terminal signals.
func invalidArgs(err error) (string, bool, error) {
	return fmt.Sprintf("invalid arguments: %s", err.Error()), true, nil
}

func missingService(name string) (string, bool, error) {
	return fmt.Sprintf("%s service not configured", name), true, nil
}

// loadGuardedInstance loads a workflow instance by id and rejects it when it
// does not belong to projectID — a console session must never act on another
// project's instance via a caller-supplied instance_id.
func loadGuardedInstance(d Deps, projectID, instanceID string) (*model.WorkflowInstance, error) {
	wi, err := repo.NewWorkflowInstanceRepo(d.Pool, d.Clock).Get(instanceID)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(wi.ProjectID, projectID) {
		return nil, fmt.Errorf("instance does not belong to this project")
	}
	return wi, nil
}
