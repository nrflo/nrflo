import { apiGet, apiPost, apiPatch, apiDelete } from './client'

export interface CLIModel {
  id: string
  cli_type: string
  display_name: string
  mapped_model: string
  reasoning_effort: string
  context_length: number
  read_only: boolean
  created_at: string
  updated_at: string
}

export interface CreateCLIModelRequest {
  id: string
  cli_type: string
  display_name: string
  mapped_model: string
  reasoning_effort?: string
  context_length?: number
}

export interface UpdateCLIModelRequest {
  display_name?: string
  mapped_model?: string
  reasoning_effort?: string
  context_length?: number
}

export async function listCLIModels(): Promise<CLIModel[]> {
  return apiGet<CLIModel[]>('/api/v1/cli-models')
}

export async function getCLIModel(id: string): Promise<CLIModel> {
  return apiGet<CLIModel>(`/api/v1/cli-models/${encodeURIComponent(id)}`)
}

export async function createCLIModel(req: CreateCLIModelRequest): Promise<CLIModel> {
  return apiPost<CLIModel>('/api/v1/cli-models', req)
}

export async function updateCLIModel(id: string, req: UpdateCLIModelRequest): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(`/api/v1/cli-models/${encodeURIComponent(id)}`, req)
}

export async function deleteCLIModel(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/cli-models/${encodeURIComponent(id)}`)
}
