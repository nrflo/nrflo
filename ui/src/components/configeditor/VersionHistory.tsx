import { formatDateTime } from '@/lib/utils'
import { Button } from '@/components/ui/Button'
import type { ConfigVersion } from '@/types/config_file'

interface Props {
  versions: ConfigVersion[]
  currentVersion: number
  onRollback: (version: number) => void
  isRollingBack?: boolean
}

export function VersionHistory({ versions, currentVersion, onRollback, isRollingBack }: Props) {
  if (versions.length === 0) {
    return <p className="text-sm text-muted-foreground">No version history.</p>
  }
  return (
    <div className="border border-border rounded-lg divide-y">
      {versions.map((v) => (
        <div key={v.version} className="flex items-center justify-between px-4 py-3 gap-3">
          <div className="min-w-0">
            <div className="text-sm font-medium flex items-center gap-2">
              <span>v{v.version}</span>
              {v.version === currentVersion && (
                <span className="text-xs bg-primary/10 text-primary rounded px-1.5 py-0.5">
                  current
                </span>
              )}
            </div>
            <div className="text-xs text-muted-foreground">
              {v.actor} · {formatDateTime(v.created_at)}
            </div>
          </div>
          {v.version !== currentVersion && (
            <Button
              variant="outline"
              size="sm"
              onClick={() => onRollback(v.version)}
              disabled={isRollingBack}
            >
              Rollback
            </Button>
          )}
        </div>
      ))}
    </div>
  )
}
