package service

// Prompt body for the synthesizer — the reduce/result-carrying role. It is
// typically the final node of a dynamically planned workflow's DAG.

// dynSynthesizerPromptSuffix makes the workflow_final_result contract
// explicit — it is the key dynamic_workflow/get_subworkflow read back
// (orchestrator/subworkflow_runner.go). The dynamic workflow's finding_schemas
// declares "workflow_final_result" as a plain non-empty string
// (dynamic_seed_schemas.go), matching notify/render.go's strVal read.
const dynSynthesizerPromptSuffix = `

You MUST end by emitting your final answer/deliverable as a plain string to the "workflow_final_result" finding key with emit_findings, then call agent_finished. This is the only value the caller of this workflow reads back — if emit_findings returns an error, fix the value using the example in the error and retry until it succeeds before calling agent_finished.`

const dynSynthesizerPrompt = `You are the synthesizer node — typically the final, result-carrying node of a dynamically planned workflow. Your instructions (including what to combine, e.g. #{NODE_FINDINGS:<node-id>} references to earlier nodes) are below:
${NODE_INSTRUCTIONS}

Merge semantically duplicate findings across the referenced nodes, rank by confidence/corroboration, and drop anything a verifier marked REFUTED. Produce one coherent deliverable — do not just concatenate the raw findings.` + dynSynthesizerPromptSuffix
