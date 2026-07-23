import type { ActiveAgentV4 } from '@/types/workflow'

export function supportsTakeControl(agent: ActiveAgentV4): boolean {
  return (
    !!agent.session_id &&
    !agent.result &&
    (agent.cli === 'claude' || agent.cli === 'codex') &&
    agent.effective_mode !== 'api' &&
    agent.effective_mode !== 'script'
  )
}

export function pickTakeControlTarget(
  activeAgents: Record<string, ActiveAgentV4>,
  panelAgent?: ActiveAgentV4 | null
): ActiveAgentV4 | undefined {
  if (panelAgent && supportsTakeControl(panelAgent)) {
    return panelAgent
  }
  return Object.values(activeAgents).find(supportsTakeControl)
}
