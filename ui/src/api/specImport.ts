import { apiGet } from './client'

export interface EnvVarCatalogEntry {
  name: string
  feature: string
  description: string
  required: boolean
}

export async function getEnvVarCatalog(): Promise<EnvVarCatalogEntry[]> {
  const resp = await apiGet<{ vars: EnvVarCatalogEntry[] }>('/api/v1/import/env-var-catalog')
  return resp.vars ?? []
}
