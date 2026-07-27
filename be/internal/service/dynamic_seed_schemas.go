package service

// dynFindingSchemas is the workflows.finding_schemas JSON for the `dynamic`
// workflow: one entry per finding key the seeded template catalog emits to
// (FindingsService.Emit hard-rejects any key with no schema — findings_emit.go).
// "workflow_final_result" is kept a plain non-empty string: notify/render.go
// reads it with strVal and get_subworkflow returns it as the caller-visible
// result (orchestrator/subworkflow_runner.go). The reserved "_workflow_plan"
// key is never declared here — it is server-owned (plan_schema.go
// ReservedFindingSchema) and ValidateFindingSchemas rejects declaring it.
const dynFindingSchemas = `[
{"key":"map","schema":{"type":"object","required":["summary","locations"],"properties":{"summary":{"type":"string"},"locations":{"type":"array","items":{"type":"object","required":["path"],"properties":{"path":{"type":"string"},"note":{"type":"string"}}}}}},"example":{"summary":"s","locations":[{"path":"be/internal/foo.go","note":"defines Foo"}]}},
{"key":"report","schema":{"type":"object","required":["verdict","summary"],"properties":{"verdict":{"enum":["pass","fail","concerns"]},"summary":{"type":"string"},"issues":{"type":"array","items":{"type":"string"}}}},"example":{"verdict":"pass","summary":"s","issues":[]}},
{"key":"claims","schema":{"type":"array","items":{"type":"object","required":["claim","quote","sourceUrl","sourceQuality","importance"],"properties":{"claim":{"type":"string"},"quote":{"type":"string"},"sourceUrl":{"type":"string"},"sourceQuality":{"enum":["primary","secondary","blog","forum","unreliable"]},"importance":{"enum":["central","supporting","tangential"]}}}},"example":[]},
{"key":"verdicts","schema":{"type":"array","items":{"type":"object","required":["claimRef","verdict","confidence"],"properties":{"claimRef":{"type":"string"},"verdict":{"enum":["CONFIRMED","PLAUSIBLE","REFUTED"]},"confidence":{"enum":["high","medium","low"]},"evidence":{"type":"string"}}}},"example":[]},
{"key":"work_log","schema":{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"},"changes":{"type":"array","items":{"type":"string"}}}},"example":{"summary":"s","changes":[]}},
{"key":"notes","schema":{"type":"object","required":["summary"],"properties":{"summary":{"type":"string"},"details":{"type":"array","items":{"type":"string"}}}},"example":{"summary":"s","details":[]}},
{"key":"premises","schema":{"type":"array","items":{"type":"object","required":["premise","status","falsifier","impact"],"properties":{"premise":{"type":"string"},"status":{"enum":["tested","untested","contradicted"]},"falsifier":{"type":"string"},"impact":{"enum":["decisive","material","minor"]}}}},"example":[]},
{"key":"cross_check","schema":{"type":"object","required":["agreement","summary"],"properties":{"agreement":{"enum":["agree","disagree","partial"]},"summary":{"type":"string"},"discrepancies":{"type":"array","items":{"type":"string"}}}},"example":{"agreement":"agree","summary":"s","discrepancies":[]}},
{"key":"workflow_final_result","schema":{"type":"string","minLength":1},"example":"Final deliverable text."}
]`
