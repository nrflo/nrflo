package tools_builtin

import (
	"context"
	"encoding/json"

	"be/internal/service/stepengine"
	"be/internal/spawner/apirun"
	"be/internal/spawner/apirun/provider"
)

// completeStepHandler implements complete_step, the ONLY caller of
// stepengine.Advance. Exposed exclusively to prompt_mode='stepwise' agent
// definitions (see spawner_api_registry.go's isStepwiseDef gating) — a
// full-mode agent's tool catalogue never resolves it.
type completeStepHandler struct{}

// stepCheckRunner adapts apirun.StepSession.RunStepChecks to
// stepengine.CheckRunner for a single session's Advance call.
type stepCheckRunner struct {
	steps     apirun.StepSession
	sessionID string
}

func (r stepCheckRunner) RunChecks(ctx context.Context, cmds []string) (failedIdx, exitCode int, outputTail string, err error) {
	return r.steps.RunStepChecks(ctx, r.sessionID, cmds)
}

func (completeStepHandler) Spec() provider.ToolSpec {
	return provider.ToolSpec{
		Name:        "complete_step",
		Description: "Advance the server-owned step cursor: submit the current step_id/revision plus a summary and the finding keys satisfying its required evidence. Rejected calls list exactly what is missing or invalid.",
		InputSchema: json.RawMessage(`{
"type":"object",
"properties":{
"step_id":{"type":"string"},
"revision":{"type":"integer"},
"summary":{"type":"string"},
"evidence":{
"type":"object",
"properties":{
"finding_keys":{"type":"array","items":{"type":"string"}}
},
"additionalProperties":false
}
},
"required":["step_id","revision"],
"additionalProperties":false
}`),
	}
}

func (completeStepHandler) Invoke(ctx context.Context, env apirun.ToolEnv, input json.RawMessage) (string, bool, error) {
	var args struct {
		StepID   string `json:"step_id"`
		Revision int    `json:"revision"`
		Summary  string `json:"summary"`
		Evidence struct {
			FindingKeys []string `json:"finding_keys"`
		} `json:"evidence"`
	}
	if len(input) > 0 {
		if err := json.Unmarshal(input, &args); err != nil {
			return invalidArgs(err)
		}
	}
	if env.Pool == nil {
		return missingService("pool")
	}
	if env.WorkflowInstanceID == "" || env.NodeID == "" {
		return missingService("step cursor (workflow_instance_id/node_id)")
	}

	contextTokens, rotateThreshold := 0, 0
	var checks stepengine.CheckRunner
	if env.Steps != nil {
		contextTokens, rotateThreshold = env.Steps.RotateSignals(env.SessionID)
		// Built only when env.Steps is non-nil, and kept as a nil
		// stepengine.CheckRunner otherwise — a typed-nil interface value
		// would defeat Advance's `e.checks != nil` skip.
		checks = stepCheckRunner{steps: env.Steps, sessionID: env.SessionID}
	}

	engine := stepengine.New(env.Pool, env.Clock, checks)
	outcome, err := engine.Advance(ctx, env.WorkflowInstanceID, env.NodeID, args.StepID, args.Revision, stepengine.Evidence{
		SessionID:       env.SessionID,
		Summary:         args.Summary,
		FindingKeys:     args.Evidence.FindingKeys,
		ContextTokens:   contextTokens,
		RotateThreshold: rotateThreshold,
	})
	if err != nil {
		return err.Error(), true, nil
	}

	switch outcome.Kind {
	case stepengine.OutcomeRejected:
		return renderRejected(env, engine, args.StepID, outcome)
	case stepengine.OutcomeNext:
		return renderNext(env, engine, outcome)
	case stepengine.OutcomeDone:
		return renderDone(outcome)
	case stepengine.OutcomeRotate:
		return renderRotate(env)
	default:
		return "unknown outcome", true, nil
	}
}
