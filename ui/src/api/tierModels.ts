import { apiGet, apiPut } from './client'

// TierModel is one ordered entry in a tier's fallback chain. Position 0 is
// the chain's primary entry; mirrors be/internal/model/tier_model.go.
export interface TierModel {
  tier: number
  position: number
  provider: string
  execution_mode: 'cli_interactive' | 'api'
  model_id: string
  reasoning_effort: string
}

export interface SetTierChainEntry {
  execution_mode: 'cli_interactive' | 'api'
  model_id: string
  reasoning_effort: string
}

export async function listTierModels(): Promise<TierModel[]> {
  return apiGet<TierModel[]>('/api/v1/tier-models')
}

export async function setTierChain(tier: number, entries: SetTierChainEntry[]): Promise<{ status: string }> {
  return apiPut<{ status: string }>(`/api/v1/tier-models/${tier}`, { entries })
}
