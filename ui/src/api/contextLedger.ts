import type { ContextLedgerSnapshot } from '@/types/contextLedger'

// GET /api/v1/sessions/{id}/context-ledger — mirrors fetchSessionPrompt
// (api/agents.ts): raw fetch (no X-Project; the endpoint's project check is
// optional), null on 404 (ledger dropped once the session ends/finishes).
export async function fetchSessionContextLedger(sessionId: string): Promise<ContextLedgerSnapshot | null> {
  const response = await fetch(`/api/v1/sessions/${sessionId}/context-ledger`, { method: 'GET' })
  if (response.status === 404) return null
  if (!response.ok) throw new Error(`Failed to fetch context ledger: ${response.status}`)
  return response.json()
}
