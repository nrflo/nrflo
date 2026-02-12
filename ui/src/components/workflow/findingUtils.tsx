import type { WorkflowFindings } from '@/types/workflow'

// Simple finding value renderer - shows strings as-is or JSON-formatted for objects
export function SimpleFindingValue({ value }: { value: unknown }): React.ReactNode {
  if (typeof value === 'string') {
    const formatted = tryFormatAsJson(value)
    if (formatted !== null) {
      return (
        <pre className="text-xs font-mono whitespace-pre-wrap break-words">
          {formatted}
        </pre>
      )
    }
    return (
      <span className="text-green-700 dark:text-green-400 whitespace-pre-wrap break-words">
        {value}
      </span>
    )
  }

  if (typeof value === 'object' && value !== null) {
    return (
      <pre className="text-xs font-mono whitespace-pre-wrap break-words">
        {JSON.stringify(value, null, 2)}
      </pre>
    )
  }

  if (value === null || value === undefined) {
    return <span className="text-muted-foreground italic">null</span>
  }
  return <span>{String(value)}</span>
}

// Try to parse a string as JSON, returning the formatted string or null if not JSON
export function tryFormatAsJson(value: string): string | null {
  try {
    const parsed = JSON.parse(value)
    return JSON.stringify(parsed, null, 2)
  } catch {
    return null
  }
}

// Check if an object looks like it contains model keys (e.g., "claude:opus", "opencode:gpt")
// rather than actual finding keys (e.g., "l1_issues", "summary")
export function isModelKeyedObject(obj: Record<string, unknown>): boolean {
  const keys = Object.keys(obj)
  if (keys.length === 0) return false
  return keys.every(key =>
    key.includes(':') ||
    key.startsWith('claude') ||
    key.startsWith('opencode') ||
    key.startsWith('openai')
  )
}

// Resolve findings for an agent - collects ALL findings including from nested model structures
// Format 1 (composite key): findings["agent_type:model_id"] = { key: value }
// Format 2 (model-keyed): findings[agent_type][model_id] = { key: value }
// Format 3 (direct): findings[agent_type] = { key: value }
export function resolveAgentFindings(
  findings: WorkflowFindings | undefined,
  agentType: string,
  modelId?: string
): Record<string, unknown> | undefined {
  if (!findings) return undefined

  // Format 1: Try composite key first (agent_type:model_id)
  if (modelId) {
    const compositeKey = `${agentType}:${modelId}`
    const compositeFindings = findings[compositeKey]
    if (compositeFindings && typeof compositeFindings === 'object') {
      return compositeFindings as Record<string, unknown>
    }
  }

  const agentFindings = findings[agentType]
  if (!agentFindings || typeof agentFindings !== 'object') return undefined

  // Format 2: Try model-specific nested findings
  if (modelId) {
    const modelFindings = agentFindings[modelId]
    if (modelFindings && typeof modelFindings === 'object' && !Array.isArray(modelFindings)) {
      return modelFindings as Record<string, unknown>
    }
  }

  // Check if agentFindings is model-keyed (contains model IDs as keys, not actual findings)
  if (isModelKeyedObject(agentFindings as Record<string, unknown>)) {
    // Merge all findings from all models into a single object
    const merged: Record<string, unknown> = {}
    for (const [modelKey, modelData] of Object.entries(agentFindings)) {
      if (modelData && typeof modelData === 'object' && !Array.isArray(modelData)) {
        for (const [findingKey, findingValue] of Object.entries(modelData as Record<string, unknown>)) {
          const keys = Object.keys(agentFindings)
          if (keys.length === 1) {
            merged[findingKey] = findingValue
          } else {
            const shortModel = modelKey.includes(':') ? modelKey.split(':')[1] : modelKey
            merged[`[${shortModel}] ${findingKey}`] = findingValue
          }
        }
      }
    }
    return Object.keys(merged).length > 0 ? merged : undefined
  }

  // Format 3: Return the agentFindings directly (it contains actual findings)
  return agentFindings as Record<string, unknown>
}
