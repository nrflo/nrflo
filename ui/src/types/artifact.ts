export interface Artifact {
  id: string
  project_id: string
  workflow_instance_id: string
  name: string
  type: string
  size_bytes: number
  content_type?: string
  source: 'input' | 'agent'
  created_by_session?: string
  created_at: string
}

export interface ArtifactUploadResponse {
  upload_id: string
  name: string
}

export interface InputArtifactRef {
  upload_id: string
  name?: string
}
