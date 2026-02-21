import { Zap, Activity } from 'lucide-react'
import { useUsageLimits } from '@/hooks/useUsageLimits'
import { Tooltip } from '@/components/ui/Tooltip'
import type { ToolUsage } from '@/types/usageLimits'

function pctColor(pct: number): string {
  if (pct > 80) return 'text-red-500'
  if (pct >= 50) return 'text-yellow-500'
  return 'text-green-500'
}

function ToolSection({ label, icon: Icon, usage }: { label: string; icon: typeof Zap; usage: ToolUsage }) {
  if (!usage.available || (!usage.session && !usage.weekly)) return null

  return (
    <div className="flex items-center gap-1">
      <Icon className="h-3.5 w-3.5" />
      <span>{label}:</span>
      {usage.session && (
        <Tooltip text={`Resets at ${usage.session.resets_at}`} placement="bottom">
          <span className={pctColor(usage.session.used_pct)}>
            {Math.round(usage.session.used_pct)}% 5h
          </span>
        </Tooltip>
      )}
      {usage.session && usage.weekly && <span>·</span>}
      {usage.weekly && (
        <Tooltip text={`Resets at ${usage.weekly.resets_at}`} placement="bottom">
          <span className={pctColor(usage.weekly.used_pct)}>
            {Math.round(usage.weekly.used_pct)}% wk
          </span>
        </Tooltip>
      )}
    </div>
  )
}

export function UsageLimits() {
  const { data, isLoading } = useUsageLimits()

  if (isLoading || !data) return null
  if (!data.claude.available && !data.codex.available) return null

  return (
    <div className="hidden sm:flex items-center gap-3 text-xs text-muted-foreground">
      <ToolSection label="Claude" icon={Zap} usage={data.claude} />
      <ToolSection label="Codex" icon={Activity} usage={data.codex} />
    </div>
  )
}
