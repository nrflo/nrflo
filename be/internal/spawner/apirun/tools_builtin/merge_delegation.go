package tools_builtin

import (
	"context"
	"encoding/json"
	"strings"

	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

type mergeDelegationHandler struct{}

func (mergeDelegationHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "merge_delegation",
		Description: "Merge an isolated delegation's server-committed branch into the live checkout's current branch, server-side. This is THE way to land executor-tier results — never merge the branch by hand via bash. Preconditions enforced server-side: the delegation must be collected (not running), must carry a branch, and the live tree must be clean. Idempotent (an already-merged branch reports already_merged). On conflict the merge is aborted, the branch is preserved for manual resolution, and the error names the conflicted files. Returns {delegation_id, status:\"merged\", branch, merge_commit}.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"delegation_id":{"type":"string"}
},
"required":["delegation_id"],
"additionalProperties":false
}`),
	}
}

func (mergeDelegationHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		DelegationID string `json:"delegation_id"`
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return invalidArgs(err)
	}
	if env.Delegator == nil {
		return missingService("delegator")
	}
	if strings.TrimSpace(args.DelegationID) == "" {
		return "delegation_id is required", true, nil
	}
	result, err := env.Delegator.MergeDelegation(ctx, env.SessionID, args.DelegationID)
	if err != nil {
		return err.Error(), true, nil
	}
	return result, false, nil
}
