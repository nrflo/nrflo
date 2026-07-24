import type { ActiveAgentV4 } from '@/types/workflow'
import { cliFromModelId } from './modelId'

export function supportsTakeControl(agent: ActiveAgentV4): boolean {
  const cli = cliFromModelId(agent.model_id)
  return (
    !!agent.session_id &&
    !agent.result &&
    (cli === 'claude' || cli === 'codex') &&
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
