// Pure, DOM-free client-side mirror of
// be/internal/service/agent_definition_steps.go's validateStepDefinitions.
// Keeps invalid stepwise configs from round-tripping to the backend just to
// be rejected; the backend remains the source of truth for these rules.
import type { StepDefinition } from '@/types/workflow'

// Mirrors model.ValidFindingSchema (be/internal/model/agent_step.go).
export const STEP_FINDING_SCHEMAS = ['nonempty_text', 'ordered_lines', 'json_array_path_change'] as const
export type FindingSchemaName = (typeof STEP_FINDING_SCHEMAS)[number]

const STEP_ID_PATTERN = /^[a-z0-9][a-z0-9_-]{0,63}$/

function validateFindingKey(stepId: string, field: string, key: string): string | null {
  const trimmed = key.trim()
  if (trimmed === '' || trimmed !== key || /[ \t\n]/.test(key)) {
    return `steps[${stepId}]: ${field} key must be non-empty and whitespace-free`
  }
  if (key.length > 128) {
    return `steps[${stepId}]: ${field} key exceeds 128 bytes`
  }
  return null
}

function validateRequiredFindings(stepId: string, findings: StepDefinition['required_findings']): string | null {
  const list = findings ?? []
  if (list.length > 20) return `steps[${stepId}]: too many required_findings (max 20)`
  for (const f of list) {
    const keyErr = validateFindingKey(stepId, 'required_findings', f.key)
    if (keyErr) return keyErr
    if (!(STEP_FINDING_SCHEMAS as readonly string[]).includes(f.schema)) {
      return `steps[${stepId}]: invalid required_findings schema "${f.schema}"`
    }
  }
  return null
}

function validateStepChecks(stepId: string, checks: string[] | undefined): string | null {
  const list = checks ?? []
  if (list.length > 20) return `steps[${stepId}]: too many checks (max 20)`
  for (const c of list) {
    if (c.trim() === '') return `steps[${stepId}]: checks entry is empty or whitespace-only`
    if (c.length > 1024) return `steps[${stepId}]: checks entry exceeds 1024 bytes`
  }
  return null
}

function validatePathOverlap(stepId: string, overlap: StepDefinition['path_overlap']): string | null {
  if (!overlap) return null
  if (overlap.left.length === 0 || overlap.right.length === 0) {
    return `steps[${stepId}]: path_overlap left and right must both be non-empty`
  }
  const seen = new Map<string, string>()
  for (const side of [
    { name: 'left', keys: overlap.left },
    { name: 'right', keys: overlap.right },
  ] as const) {
    if (side.keys.length > 10) return `steps[${stepId}]: path_overlap ${side.name} has too many keys (max 10)`
    for (const key of side.keys) {
      const keyErr = validateFindingKey(stepId, `path_overlap ${side.name}`, key)
      if (keyErr) return keyErr
      const other = seen.get(key)
      if (other && other !== side.name) {
        return `steps[${stepId}]: path_overlap key "${key}" appears in both left and right`
      }
      seen.set(key, side.name)
    }
  }
  return null
}

// validateStepDefinitions returns all violations found (empty = valid), each
// message mirroring the backend's validationErrorf wording so the reported
// error is recognizable across the client/server boundary.
export function validateStepDefinitions(steps: StepDefinition[]): string[] {
  const errors: string[] = []
  if (steps.length === 0) {
    errors.push('steps: at least one step is required')
    return errors
  }
  if (steps.length > 20) errors.push('steps: too many entries (max 20)')

  const seen = new Set<string>()
  for (const step of steps) {
    const stepId = step.step_id
    if (!STEP_ID_PATTERN.test(stepId)) {
      errors.push(`steps: invalid step_id "${stepId}" (must match ^[a-z0-9][a-z0-9_-]{0,63}$)`)
    } else if (seen.has(stepId)) {
      errors.push(`steps: duplicate step_id "${stepId}"`)
    } else {
      seen.add(stepId)
    }
    if (step.title.trim() === '') errors.push(`steps[${stepId}]: title is required`)
    if (step.instruction.trim() === '') errors.push(`steps[${stepId}]: instruction is required`)
    if (step.instruction.length > 16384) errors.push(`steps[${stepId}]: instruction exceeds 16384 bytes`)

    const findingsErr = validateRequiredFindings(stepId, step.required_findings)
    if (findingsErr) errors.push(findingsErr)
    const checksErr = validateStepChecks(stepId, step.checks)
    if (checksErr) errors.push(checksErr)
    const overlapErr = validatePathOverlap(stepId, step.path_overlap)
    if (overlapErr) errors.push(overlapErr)
  }
  return errors
}
