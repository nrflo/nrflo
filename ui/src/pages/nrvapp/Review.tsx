import { useSearchParams } from 'react-router-dom'
import { ClipboardList } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { useReviewItems } from '@/hooks/useNrvapp'
import type { ReviewStatus } from '@/types/nrvapp'
import { cn } from '@/lib/utils'

const STATUS_TABS: { label: string; value: ReviewStatus | 'all' }[] = [
  { label: 'Pending', value: 'pending' },
  { label: 'Approved', value: 'approved' },
  { label: 'Rejected', value: 'rejected' },
  { label: 'All', value: 'all' },
]

const STATUS_BADGE: Record<string, 'secondary' | 'default' | 'destructive'> = {
  pending: 'secondary',
  approved: 'default',
  rejected: 'destructive',
}

export function ReviewPage() {
  const [params, setParams] = useSearchParams()
  const status = (params.get('status') ?? 'pending') as ReviewStatus | 'all'
  const { data = [], isLoading, error } = useReviewItems(status === 'all' ? undefined : status)

  return (
    <div className="p-6 max-w-4xl mx-auto space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <ClipboardList className="h-5 w-5" />
            Review Queue
          </CardTitle>
        </CardHeader>
        <CardContent>
          <div className="flex gap-2 mb-4">
            {STATUS_TABS.map((tab) => (
              <button
                key={tab.value}
                type="button"
                onClick={() => setParams({ status: tab.value })}
                className={cn(
                  'px-3 py-1.5 text-sm rounded-md transition-colors',
                  status === tab.value
                    ? 'bg-muted text-foreground font-medium'
                    : 'text-muted-foreground hover:bg-muted/50'
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>
          {isLoading && <p className="text-center py-8 text-muted-foreground">Loading…</p>}
          {error && (
            <p className="text-center py-8 text-destructive">{(error as Error).message}</p>
          )}
          {!isLoading && !error && data.length === 0 && (
            <p className="text-center py-8 text-muted-foreground">No review items.</p>
          )}
          {data.length > 0 && (
            <div className="border border-border rounded-lg divide-y">
              {data.map((item) => (
                <Link
                  key={item.id}
                  to={`/nrvapp/review/${item.id}`}
                  className="flex items-center justify-between px-4 py-3 hover:bg-muted/50 transition-colors"
                >
                  <div>
                    <div className="text-sm font-medium">{item.tool_name}</div>
                    <div className="text-xs text-muted-foreground">{item.created_at}</div>
                  </div>
                  <Badge variant={STATUS_BADGE[item.status] ?? 'secondary'}>{item.status}</Badge>
                </Link>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
