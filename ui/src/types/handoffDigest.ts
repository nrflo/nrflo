// Mirrors be/internal/model/refinery_digest.go (snake_case JSON) for the
// current autonomous-slot digest of a workflow instance's node/session.

// GET /api/v1/sessions/{id}/handoff-digest response.
export interface HandoffDigest {
  content: string
  version: number
  fold_count: number
  updated_at: string
}

// agent.handoff_digest WS payload — the durable digest broadcast after the
// refinery sidecar's debounced Upsert, scoped to the originating session.
export interface HandoffDigestEvent extends HandoffDigest {
  session_id: string
}
