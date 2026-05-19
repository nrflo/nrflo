import { apiGet, apiDelete, apiUploadMultipart } from './client'
import { useConnectionsStore } from '@/stores/connectionsStore'
import type { Artifact, ArtifactUploadResponse } from '@/types/artifact'

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
  const baseURL = useConnectionsStore.getState().active().baseURL
  return `${baseURL}/api/v1/artifacts/${encodeURIComponent(id)}/download`
}

export function deleteArtifact(id: string): Promise<void> {
  return apiDelete<void>(`/api/v1/artifacts/${encodeURIComponent(id)}`)
}
