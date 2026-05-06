import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type { PythonScript, PythonScriptCreateRequest, PythonScriptUpdateRequest, ValidationResult, BrowseResponse, ReadFileResponse } from '@/types/pythonScript'

export async function listPythonScripts(): Promise<PythonScript[]> {
  return apiGet<PythonScript[]>('/api/v1/python-scripts')
}

export async function getPythonScript(id: string): Promise<PythonScript> {
  return apiGet<PythonScript>(`/api/v1/python-scripts/${encodeURIComponent(id)}`)
}

export async function createPythonScript(data: PythonScriptCreateRequest): Promise<PythonScript> {
  return apiPost<PythonScript>('/api/v1/python-scripts', data)
}

export async function updatePythonScript(
  id: string,
  data: PythonScriptUpdateRequest
): Promise<{ status: string }> {
  return apiPatch<{ status: string }>(`/api/v1/python-scripts/${encodeURIComponent(id)}`, data)
}

export async function deletePythonScript(id: string): Promise<{ status: string }> {
  return apiDelete<{ status: string }>(`/api/v1/python-scripts/${encodeURIComponent(id)}`)
}

export async function validatePythonScript(code: string): Promise<ValidationResult> {
  return apiPost<ValidationResult>('/api/v1/python-scripts/validate', { code })
}

export async function browseDirectory(path?: string): Promise<BrowseResponse> {
  const qs = path ? `?path=${encodeURIComponent(path)}` : ''
  return apiGet<BrowseResponse>(`/api/v1/python-scripts/browse${qs}`)
}

export async function readPythonFile(path: string): Promise<ReadFileResponse> {
  return apiGet<ReadFileResponse>(`/api/v1/python-scripts/read-file?path=${encodeURIComponent(path)}`)
}
