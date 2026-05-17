import { apiGet, apiDelete, apiUploadMultipart } from './client'
import type { Artifact, ArtifactUploadResponse } from '@/types/artifact'

const API_BASE_URL = import.meta.env.VITE_API_URL || ''

export function uploadArtifact(file: File): Promise<ArtifactUploadResponse> {
  return apiUploadMultipart<ArtifactUploadResponse>('/api/v1/artifact-uploads', file)
}

export function cancelUpload(uploadId: string): Promise<void> {
  return apiDelete<void>(`/api/v1/artifact-uploads/${encodeURIComponent(uploadId)}`)
}

export function listArtifacts(wfiId: string): Promise<Artifact[]> {
  return apiGet<Artifact[]>(`/api/v1/workflow-instances/${encodeURIComponent(wfiId)}/artifacts`)
}

export function downloadArtifactURL(id: string): string {
  return `${API_BASE_URL}/api/v1/artifacts/${encodeURIComponent(id)}/download`
}

export function deleteArtifact(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/artifacts/${encodeURIComponent(id)}`)
}
