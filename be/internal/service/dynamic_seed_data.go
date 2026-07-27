package service

// Bundled definition of the global `dynamic` workflow's template catalog.
// Seeded under GlobalProjectID at startup by EnsureGlobalDynamicWorkflow. Edit
// here (and the sibling dynamic_seed_prompts_*.go / dynamic_seed_planner.go
// files) to change the shipped definition; existing seeded copies are not
// overwritten (the seed is create-if-absent).
//
// dynamic-planner is node_role='planner' — it is NOT a fanout_template, so it
// never appears in the plan catalog (AllowedTemplates filters on
// node_role='fanout_template') and never executes as a phase (ListExecutable
// filters on node_role='static'). Seeding it here overrides the system
// planner for the `dynamic` workflow only (orchestrator/planner.go
// resolvePlannerDef prefers a workflow-local node_role='planner' def).
// Every other entry is node_role='fanout_template': general-purpose building
// blocks a planner can bind plan nodes to, spanning exploration, review,
// implementation, research, verification, cross-checking, and synthesis. All
// run cli_interactive (self-authenticating CLIs), so the workflow needs no
// server-side API credential.

// dynAgent describes one seeded agent definition: a fanout_template (the
// default when NodeRole is empty) or the workflow-local planner override.
type dynAgent struct {
	ID              string
	Model           string
	ReasoningEffort string
	Tools           string
	NodeRole        string // "" defaults to fanout_template at seed time
	Description     string
	FindingKey      string // finding key this template emits to; "" for the planner
}

// dynAgents is the seeded roster: the workflow-local planner override plus
// the 10 fanout_template building blocks. Node ids in a plan's manifest
// reference the fanout_template entries by `template`. Non-codex worker/
// verifier templates default ReasoningEffort to "low" (synthesizer
// "medium") — a cheap-tier default that is the soft complement to the
// EnforcePremiumWorkerCap guardrail (plan_validate_premium.go), which caps
// premium (opus/fable) nodes server-side regardless of what the planner picks.
var dynAgents = []dynAgent{
	{
		ID:          "dynamic-planner",
		Model:       "opus-5",
		Tools:       "emit_findings",
		NodeRole:    "planner",
		Description: "Workflow-local planner for the dynamic workflow: decomposes a goal into a layered manifest bound to the templates below.",
	},
	{
		ID:              "codebase-explorer",
		Model:           "haiku-4-5",
		ReasoningEffort: "low",
		Tools:           "findings_add,findings_get,artifact_get",
		Description:     "Fast, lean-context codebase exploration: locates files/symbols/patterns and reports back without editing anything. Read-only by prompt discipline, not sandboxing. Emits to finding key `map`.",
		FindingKey:      "map",
	},
	{
		ID:              "module-reviewer",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "emit_findings,findings_get,artifact_get,artifact_list,read_document",
		Description:     "Reviews a module, change, or another node's finding critically and reports a pass/fail/concerns verdict; read-only by prompt discipline. Emits to finding key `report`.",
		FindingKey:      "report",
	},
	{
		ID:              "module-reviewer-codex",
		Model:           "gpt-5.6-terra",
		ReasoningEffort: "high",
		Tools:           "emit_findings,findings_get,artifact_get,artifact_list,read_document",
		Description:     "Provider-diverse twin of module-reviewer (codex GPT-5.6 Terra) for a cross-provider review quorum. Read-only by prompt discipline. Emits to finding key `report`.",
		FindingKey:      "report",
	},
	{
		ID:              "implementor-worker",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "*",
		Description:     "General-purpose implementation node with full tool access — writes code, runs commands, edits files in the shared working tree. Emits to finding key `work_log`.",
		FindingKey:      "work_log",
	},
	{
		ID:              "web-researcher",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "web_search,web_fetch,read_document,artifact_get,artifact_list,emit_findings",
		Description:     "Web research node: searches, fetches, and extracts falsifiable claims with verbatim quotes and sources. Emits to finding key `claims`.",
		FindingKey:      "claims",
	},
	{
		ID:              "web-researcher-cheap",
		Model:           "haiku-4-5",
		ReasoningEffort: "low",
		Tools:           "web_search,web_fetch,read_document,artifact_get,artifact_list,emit_findings",
		Description:     "Cheap-tier twin of web-researcher (haiku): identical claim-extraction contract at a fraction of the cost. The default binding for wide research fan-outs — reach for web-researcher only when a node needs deeper reasoning over what it reads. Emits to finding key `claims`.",
		FindingKey:      "claims",
	},
	{
		ID:              "premise-auditor",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "web_search,web_fetch,emit_findings",
		Description:     "Attacks the goal itself rather than the findings: names assumptions the plan treats as settled but never tests, and the measurement that would falsify each. Bind it in the FIRST layer alongside the research nodes, never after them, so it is not anchored by their output. Emits to finding key `premises`.",
		FindingKey:      "premises",
	},
	{
		ID:              "finding-verifier",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "emit_findings,findings_get,web_search",
		Description:     "Adversarial, refute-oriented verifier: checks an earlier node's findings and returns a CONFIRMED|PLAUSIBLE|REFUTED verdict per item (plausible-by-default). Emits to finding key `verdicts`.",
		FindingKey:      "verdicts",
	},
	{
		ID:              "finding-verifier-codex",
		Model:           "gpt-5.6-sol",
		ReasoningEffort: "high",
		Tools:           "emit_findings,findings_get,web_search",
		Description:     "Provider-diverse twin of finding-verifier (codex GPT-5.6 Sol) for a cross-provider verification quorum. Emits to finding key `verdicts`.",
		FindingKey:      "verdicts",
	},
	{
		ID:              "generic-worker",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "findings_add,findings_get,artifact_get,artifact_list,read_document",
		Description:     "Moderate, read-leaning general-purpose node for tasks that don't fit the other templates (analysis, drafting, light investigation). Emits to finding key `notes`.",
		FindingKey:      "notes",
	},
	{
		ID:              "cross-checker",
		Model:           "sonnet-5",
		ReasoningEffort: "low",
		Tools:           "emit_findings,findings_get",
		Description:     "Reads two prior nodes' findings (bind via #{NODE_FINDINGS:<a>} / #{NODE_FINDINGS:<b>} in its instructions) and reports where they agree or disagree. Emits to finding key `cross_check`.",
		FindingKey:      "cross_check",
	},
	{
		ID:              "synthesizer",
		Model:           "opus-5",
		ReasoningEffort: "medium",
		Tools:           "emit_findings,findings_get",
		Description:     "Final, result-carrying node: merges semantic duplicates across earlier findings, ranks by confidence, and emits exactly once. Emits to finding key `workflow_final_result`.",
		FindingKey:      "workflow_final_result",
	},
}

// dynPrompt returns the seeded prompt body for a dynAgents entry id.
func dynPrompt(id string) string {
	switch id {
	case "dynamic-planner":
		return dynPlannerPrompt
	case "codebase-explorer":
		return dynExplorerPrompt
	case "module-reviewer", "module-reviewer-codex":
		return dynReviewerPrompt
	case "implementor-worker":
		return dynImplementorPrompt
	case "web-researcher", "web-researcher-cheap":
		return dynResearcherPrompt
	case "premise-auditor":
		return dynPremiseAuditorPrompt
	case "finding-verifier", "finding-verifier-codex":
		return dynVerifierPrompt
	case "generic-worker":
		return dynGenericWorkerPrompt
	case "cross-checker":
		return dynCrossCheckerPrompt
	case "synthesizer":
		return dynSynthesizerPrompt
	}
	return ""
}
