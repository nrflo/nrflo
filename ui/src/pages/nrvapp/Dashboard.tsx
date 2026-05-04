import { useState } from 'react'
import { BarChart3 } from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { RangeSelector } from '@/components/nrvapp/RangeSelector'
import { SummaryCards } from '@/components/nrvapp/SummaryCards'
import { ThroughputChart } from '@/components/nrvapp/ThroughputChart'
import { EditRateChart } from '@/components/nrvapp/EditRateChart'
import { useNrvappSummary, useNrvappEditRate, useNrvappThroughput } from '@/hooks/useNrvapp'
import type { NrvappRange } from '@/types/nrvapp'

export function NrvappDashboard() {
  const [range, setRange] = useState<NrvappRange>('7d')
  const bucket = range === '7d' ? ('1h' as const) : ('6h' as const)

  const { data: summary } = useNrvappSummary(range)
  const { data: editRate = [] } = useNrvappEditRate(range)
  const { data: throughput = [] } = useNrvappThroughput(range, bucket)

  const summaryCards = summary
    ? [
        { label: 'Total Dispatches', value: summary.total_dispatches },
        { label: 'Total Reviews', value: summary.total_reviews },
        { label: 'Pending Reviews', value: summary.pending_reviews },
        { label: 'Approve Rate', value: `${Math.round(summary.approved_rate * 100)}%` },
        { label: 'Reject Rate', value: `${Math.round(summary.reject_rate * 100)}%` },
      ]
    : []

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-4">
      <div className="flex items-center justify-between">
        <h2 className="text-lg font-semibold flex items-center gap-2">
          <BarChart3 className="h-5 w-5" />
          Insights
        </h2>
        <RangeSelector value={range} onChange={setRange} />
      </div>

      {summaryCards.length > 0 && <SummaryCards cards={summaryCards} />}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-4">
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Throughput</CardTitle>
          </CardHeader>
          <CardContent className="pt-0">
            <ThroughputChart data={throughput} />
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle className="text-sm">Edit Rate by Tool</CardTitle>
          </CardHeader>
          <CardContent className="pt-0">
            <EditRateChart data={editRate} />
          </CardContent>
        </Card>
      </div>
    </div>
  )
}
