import { PlusCircle, CheckCircle2, Cpu, Clock } from 'lucide-react'
import { useDailyStats } from '@/hooks/useTickets'
import { formatTokenCount, formatDurationSec } from '@/lib/utils'

export function DailyStats() {
  const { data, isLoading } = useDailyStats()

  if (isLoading || !data) return null

  return (
    <div className="hidden sm:flex items-center gap-3 text-xs text-muted-foreground">
      <div className="flex items-center gap-1">
        <PlusCircle className="h-3.5 w-3.5" />
        <span>{data.tickets_created} created</span>
      </div>
      <div className="flex items-center gap-1">
        <CheckCircle2 className="h-3.5 w-3.5" />
        <span>{data.tickets_closed} closed</span>
      </div>
      <div className="flex items-center gap-1">
        <Cpu className="h-3.5 w-3.5" />
        <span>{formatTokenCount(data.tokens_spent)} tokens</span>
      </div>
      <div className="flex items-center gap-1">
        <Clock className="h-3.5 w-3.5" />
        <span>{formatDurationSec(data.agent_time_sec)}</span>
      </div>
    </div>
  )
}
