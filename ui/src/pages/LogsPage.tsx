import { useState } from 'react'
import { Terminal, Monitor } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { cn } from '@/lib/utils'
import { useLogs } from '@/hooks/useLogs'

type LogType = 'be' | 'fe'

const tabs: { key: LogType; label: string; icon: React.ReactNode }[] = [
  { key: 'be', label: 'BE', icon: <Terminal className="h-4 w-4" /> },
  { key: 'fe', label: 'FE', icon: <Monitor className="h-4 w-4" /> },
]

export function LogsPage() {
  const [activeTab, setActiveTab] = useState<LogType>('be')
  const { data, isLoading, error, refetch } = useLogs(activeTab)

  return (
    <div className="max-w-7xl mx-auto space-y-6">
      <h1 className="text-2xl font-bold">Logs</h1>

      <div className="border-b border-border">
        <div className="flex gap-1">
          {tabs.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={cn(
                'flex items-center gap-2 px-4 py-2 text-sm font-medium border-b-2 transition-colors',
                activeTab === tab.key
                  ? 'border-primary text-primary'
                  : 'border-transparent text-muted-foreground hover:text-foreground'
              )}
            >
              {tab.icon}
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      {isLoading && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {error && (
        <div className="text-center py-12 space-y-3">
          <p className="text-sm text-destructive">
            Failed to load logs: {error.message}
          </p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {!isLoading && !error && (
        <div className="rounded-lg border border-border overflow-auto max-h-[calc(100vh-16rem)]">
          <table className="min-w-full">
            <tbody>
              {data?.lines.length === 0 && (
                <tr>
                  <td className="px-4 py-8 text-center text-sm text-muted-foreground">
                    No log lines available.
                  </td>
                </tr>
              )}
              {data?.lines.map((line, i) => (
                <tr key={i} className="border-b border-border last:border-b-0">
                  <td className="px-4 py-1 font-mono text-sm whitespace-pre">
                    {line}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  )
}
