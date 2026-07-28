export type DocKind = 'common' | 'cli' | 'python' | 'api' | 'local-providers' | 'mcp-external'

export interface AgentManualResponse {
  content: string
  title: string
}
