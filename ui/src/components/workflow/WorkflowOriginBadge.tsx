import { Terminal } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/Badge'
import { Tooltip } from '@/components/ui/Tooltip'
import { useIsAdmin } from '@/stores/authStore'

interface WorkflowOriginBadgeProps {
  origin?: string
  originSessionId?: string
}

export function WorkflowOriginBadge({ origin, originSessionId }: WorkflowOriginBadgeProps) {
  const isAdmin = useIsAdmin()

  if (origin !== 'console') return null

  const shortId = originSessionId ? originSessionId.slice(0, 8) : ''
  const badge = (
    <Badge variant="secondary" className="inline-flex items-center gap-1">
      <Terminal className="h-3 w-3" />
      Console
    </Badge>
  )
  const tooltipped = (
    <Tooltip text={`Launched from console session ${shortId}`}>{badge}</Tooltip>
  )

  if (isAdmin && originSessionId) {
    return (
      <Link to={`/console?session=${encodeURIComponent(originSessionId)}`}>
        {tooltipped}
      </Link>
    )
  }

  return tooltipped
}
