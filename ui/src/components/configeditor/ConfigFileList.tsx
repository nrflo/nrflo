import { useMemo } from 'react'
import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/Badge'
import type { ConfigFileMeta } from '@/types/config_file'

interface Props {
  files: ConfigFileMeta[]
}

export function ConfigFileList({ files }: Props) {
  const grouped = useMemo(() => {
    const map = new Map<string, ConfigFileMeta[]>()
    for (const f of files) {
      const dir = f.path.includes('/') ? f.path.split('/').slice(0, -1).join('/') : '.'
      const existing = map.get(dir) ?? []
      existing.push(f)
      map.set(dir, existing)
    }
    return Array.from(map.entries()).sort(([a], [b]) => a.localeCompare(b))
  }, [files])

  return (
    <div className="space-y-4">
      {grouped.map(([dir, items]) => (
        <div key={dir}>
          <div className="text-xs font-medium text-muted-foreground px-1 mb-1">
            {dir === '.' ? 'Root' : dir}
          </div>
          <div className="border border-border rounded-lg divide-y">
            {items.map((file) => (
              <Link
                key={file.path}
                to={`/config-files/${encodeURIComponent(file.path)}`}
                className="flex items-center justify-between px-4 py-3 hover:bg-muted/50 transition-colors"
              >
                <div>
                  <div className="text-sm font-medium">{file.path.split('/').pop()}</div>
                  <div className="text-xs text-muted-foreground">v{file.latest_version}</div>
                </div>
                {file.has_schema && (
                  <Badge variant="secondary" className="text-xs">
                    Schema
                  </Badge>
                )}
              </Link>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}
