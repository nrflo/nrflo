import { RefreshCw } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { useAgentManual } from '@/hooks/useDocs'

export function DocumentationPage() {
  const { data, isLoading, error, refetch, isFetching } = useAgentManual()

  return (
    <div className="max-w-7xl mx-auto p-6 space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold">Agent Documentation</h1>
        <Button
          variant="outline"
          size="sm"
          onClick={() => refetch()}
          disabled={isFetching}
        >
          <RefreshCw className={`h-4 w-4 mr-2 ${isFetching ? 'animate-spin' : ''}`} />
          Refresh
        </Button>
      </div>

      {isLoading && (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      )}

      {error && (
        <div className="text-center py-12 space-y-3">
          <p className="text-sm text-destructive">
            Failed to load documentation: {error.message}
          </p>
          <Button variant="outline" size="sm" onClick={() => refetch()}>
            Retry
          </Button>
        </div>
      )}

      {!isLoading && !error && data && (
        <div className="markdown-content rounded-lg border border-border p-6">
          <ReactMarkdown
            components={{
              h1: ({ children }) => (
                <h1 className="text-2xl font-bold mt-6 mb-4 first:mt-0">{children}</h1>
              ),
              h2: ({ children }) => (
                <h2 className="text-xl font-semibold mt-5 mb-3">{children}</h2>
              ),
              h3: ({ children }) => (
                <h3 className="text-lg font-semibold mt-4 mb-2">{children}</h3>
              ),
              p: ({ children }) => (
                <p className="mb-3 leading-relaxed">{children}</p>
              ),
              ul: ({ children }) => (
                <ul className="list-disc pl-6 mb-3 space-y-1">{children}</ul>
              ),
              ol: ({ children }) => (
                <ol className="list-decimal pl-6 mb-3 space-y-1">{children}</ol>
              ),
              li: ({ children }) => (
                <li className="leading-relaxed">{children}</li>
              ),
              code: ({ className, children }) => {
                const isBlock = className?.startsWith('language-')
                if (isBlock) {
                  return (
                    <code className="block bg-muted rounded-md p-4 text-sm font-mono overflow-x-auto mb-3">
                      {children}
                    </code>
                  )
                }
                return (
                  <code className="bg-muted px-1.5 py-0.5 rounded text-sm font-mono">
                    {children}
                  </code>
                )
              },
              pre: ({ children }) => (
                <pre className="mb-3">{children}</pre>
              ),
              table: ({ children }) => (
                <div className="overflow-x-auto mb-3">
                  <table className="min-w-full border-collapse border border-border text-sm">
                    {children}
                  </table>
                </div>
              ),
              th: ({ children }) => (
                <th className="border border-border bg-muted px-3 py-2 text-left font-semibold">
                  {children}
                </th>
              ),
              td: ({ children }) => (
                <td className="border border-border px-3 py-2">{children}</td>
              ),
              blockquote: ({ children }) => (
                <blockquote className="border-l-4 border-border pl-4 italic text-muted-foreground mb-3">
                  {children}
                </blockquote>
              ),
              hr: () => <hr className="my-6 border-border" />,
              a: ({ href, children }) => (
                <a
                  href={href}
                  className="text-primary underline hover:text-primary/80"
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {children}
                </a>
              ),
            }}
          >
            {data.content}
          </ReactMarkdown>
        </div>
      )}
    </div>
  )
}
