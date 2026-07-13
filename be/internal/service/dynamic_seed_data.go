package service

// Bundled definition of the global `dynamic` workflow. Seeded under
// GlobalProjectID at startup by EnsureGlobalDynamicWorkflow. Edit here to
// change the shipped definition; existing seeded copies are not overwritten
// (the seed is create-if-absent).
//
// Unlike deep-research, this workflow carries NO static agent_definitions at
// all — every def is node_role='fanout_template', so IsPlanDriven is true and
// BuildSpawnerConfig yields zero executable phases (orchestrator_start.go
// allows this for plan-driven defs). The planner itself is not a
// workflow-local def: it resolves to the system `planner` role seeded in
// migration 000158. All templates run cli_interactive (self-authenticating
// CLIs), so the workflow needs no server-side API credential.

// dynSynthesizerPromptSuffix is appended to the synthesizer template's prompt
// to make the workflow_final_result contract explicit — it is the key
// dynamic_workflow/get_subworkflow read back (orchestrator/subworkflow_runner.go).
const dynSynthesizerPromptSuffix = `

You MUST end by writing your final answer/deliverable to the "workflow_final_result" finding key with the findings_add tool (or emit_findings if a schema applies), then call agent_finished. This is the only value the caller of this workflow reads back.`

// dynAgent describes one seeded fanout_template agent definition.
type dynAgent struct {
	ID     string
	Model  string
	Tools  string
	Prompt string
}

// dynAgents is the seeded template roster: general-purpose building blocks a
// planner can bind plan nodes to, spanning research, implementation, review,
// and synthesis. Node ids in a plan's manifest reference these by `template`.
var dynAgents = []dynAgent{
	{
		ID:    "researcher",
		Model: "sonnet",
		Tools: "web_search,web_fetch,read_document,artifact_get,artifact_list,findings_add",
		Prompt: `You are a researcher node in a dynamically planned workflow. Your instructions for this node are provided above.

Use web_search / web_fetch to gather the information you need (artifact_get / read_document if a fetched page was offloaded to an artifact). Write your findings to a well-named finding key with findings_add, then call agent_finished. If you cannot complete the task, call agent_fail with the reason.`,
	},
	{
		ID:    "worker",
		Model: "sonnet",
		Tools: "*",
		Prompt: `You are a general-purpose implementation node in a dynamically planned workflow. Your instructions for this node are provided above.

Do the work described, using whatever tools are available to you. Record what you did with findings_add under a well-named key, then call agent_finished. If you cannot complete the task, call agent_fail with the reason.`,
	},
	{
		ID:    "reviewer",
		Model: "sonnet",
		Tools: "findings_get,findings_add,web_search,web_fetch,read_document,artifact_get",
		Prompt: `You are a reviewer node in a dynamically planned workflow. Your instructions for this node (including what to review, e.g. #{NODE_FINDINGS:<node-id>} from an earlier layer) are provided above.

Review the referenced work critically, then write your verdict/feedback to a well-named finding key with findings_add, then call agent_finished. If you cannot complete the review, call agent_fail with the reason.`,
	},
	{
		ID:     "synthesizer",
		Model:  "opus_4_8",
		Tools:  "findings_get,findings_add",
		Prompt: `You are the synthesizer node — typically the final, result-carrying node of a dynamically planned workflow. Your instructions (including what to combine, e.g. #{NODE_FINDINGS:<node-id>} references to earlier layers) are provided above.` + dynSynthesizerPromptSuffix,
	},
}
