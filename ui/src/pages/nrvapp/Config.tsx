import { FolderOpen } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { ConfigFileList } from '@/components/nrvapp/ConfigFileList'
import { useConfigFiles } from '@/hooks/useNrvapp'
import { ApiError } from '@/api/client'

export function ConfigPage() {
  const { data = [], isLoading, error } = useConfigFiles()

  const is400 = error instanceof ApiError && error.status === 400

  return (
    <div className="p-6 max-w-3xl mx-auto space-y-4">
      <Card>
        <CardHeader>
          <CardTitle className="flex items-center gap-2">
            <FolderOpen className="h-5 w-5" />
            Config Files
          </CardTitle>
          <CardDescription>Customer configuration managed by Vertical app</CardDescription>
        </CardHeader>
        <CardContent>
          {isLoading && <p className="text-center py-8 text-muted-foreground">Loading…</p>}
          {is400 && (
            <div className="text-center py-8 space-y-2">
              <p className="text-muted-foreground">
                Customer config directory is not configured.
              </p>
              <Link to="/settings" className="text-sm text-primary underline">
                Go to Settings
              </Link>
            </div>
          )}
          {error && !is400 && (
            <p className="text-center py-8 text-destructive">{(error as Error).message}</p>
          )}
          {!isLoading && !error && data.length === 0 && (
            <p className="text-center py-8 text-muted-foreground">No config files found.</p>
          )}
          {!isLoading && !error && data.length > 0 && <ConfigFileList files={data} />}
        </CardContent>
      </Card>
    </div>
  )
}
