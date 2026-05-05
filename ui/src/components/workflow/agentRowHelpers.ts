import type { AgentHistoryEntry, AgentSession } from '@/types/workflow'

export function findSession(entry: AgentHistoryEntry, sessions: AgentSession[]): AgentSession | undefined {
  if (entry.session_id) {
    const byId = sessions.find(s => s.id === entry.session_id)
    if (byId) return byId
  }
  return sessions.find(s =>
    s.agent_type === entry.agent_type &&
    s.phase === entry.phase &&
    (!entry.model_id || s.model_id === entry.model_id)
  )
}
