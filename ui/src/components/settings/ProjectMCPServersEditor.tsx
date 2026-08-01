import { useEffect, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Textarea } from '@/components/ui/Textarea'
import { useMCPServers, useSetMCPServers } from '@/hooks/useProjectSettings'
import type { MCPServerSpec } from '@/api/projectSettings'

const PLACEHOLDER = `{
  "unity": { "command": "uv", "args": ["run", "mcp_server.py"] },
  "docs": { "type": "http", "url": "https://example.com/mcp" }
}`

// Edits the external_mcp_servers project config: MCP servers merged into every
// spawned CLI agent's MCP surface alongside the nrflo bridge.
export function ProjectMCPServersEditor({ projectId }: { projectId: string }) {
  const { data } = useMCPServers(projectId)
  const mutation = useSetMCPServers()
  const [text, setText] = useState('')
  const [parseError, setParseError] = useState<string | null>(null)
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    if (data && !dirty) {
      setText(data.servers ? JSON.stringify(data.servers, null, 2) : '')
    }
  }, [data, dirty])

  const handleChange = (value: string) => {
    setText(value)
    setDirty(true)
    if (value.trim() === '') {
      setParseError(null)
      return
    }
    try {
      const parsed = JSON.parse(value)
      if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
        setParseError('must be a JSON object mapping server name to spec')
      } else {
        setParseError(null)
      }
    } catch {
      setParseError('invalid JSON')
    }
  }

  const handleSave = () => {
    let servers: Record<string, MCPServerSpec> | null = null
    if (text.trim() !== '') {
      servers = JSON.parse(text)
    }
    mutation.mutate(
      { projectId, servers },
      { onSuccess: () => setDirty(false) },
    )
  }

  return (
    <div className="border-t border-border pt-3 space-y-3">
      <div className="text-sm font-medium text-muted-foreground">External MCP Servers</div>
      <p className="text-xs text-muted-foreground">
        MCP servers attached to every spawned agent alongside the nrflo bridge (name → spec: command/args/env for
        stdio, url for http/sse). Leave empty to disable.
      </p>
      <Textarea
        value={text}
        onChange={(e) => handleChange(e.target.value)}
        placeholder={PLACEHOLDER}
        rows={8}
        className="font-mono text-xs"
        aria-label="External MCP servers JSON"
      />
      {parseError && <p className="text-sm text-destructive">{parseError}</p>}
      {mutation.isError && <p className="text-sm text-destructive">{(mutation.error as Error).message}</p>}
      <Button
        type="button"
        variant="secondary"
        size="sm"
        onClick={handleSave}
        disabled={!dirty || !!parseError || mutation.isPending}
      >
        {mutation.isPending ? 'Saving…' : 'Save MCP Servers'}
      </Button>
    </div>
  )
}
