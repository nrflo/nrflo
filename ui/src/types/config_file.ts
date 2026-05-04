export interface ConfigFileMeta {
  path: string
  latest_version: number
  has_schema: boolean
  updated_at: string
}

export interface ConfigFile {
  path: string
  content: string
  schema?: Record<string, unknown>
  version: number
}

export interface ConfigVersion {
  version: number
  actor: string
  created_at: string
  content?: string
}
