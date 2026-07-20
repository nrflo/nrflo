import type { HandoffDigest } from '@/types/handoffDigest'

// GET /api/v1/sessions/{id}/handoff-digest — mirrors fetchSessionContextLedger
// (api/contextLedger.ts): raw fetch (no X-Project), null on 404. Unlike the
// context ledger, the digest is durable (refinery_autonomous_digests), so it
// also resolves for finished sessions.
export async function fetchSessionHandoffDigest(sessionId: string): Promise<HandoffDigest | null> {
  const response = await fetch(`/api/v1/sessions/${sessionId}/handoff-digest`, { method: 'GET' })
  if (response.status === 404) return null
  if (!response.ok) throw new Error(`Failed to fetch handoff digest: ${response.status}`)
  return response.json()
}
