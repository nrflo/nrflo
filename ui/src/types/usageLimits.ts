export interface UsageLimits {
  claude: ToolUsage
  codex: ToolUsage
  fetched_at: string | null
}

export interface ToolUsage {
  available: boolean
  session: UsageMetric | null
  weekly: UsageMetric | null
  error?: string
}

export interface UsageMetric {
  used_pct: number
  resets_at: string
}
