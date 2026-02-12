import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ChevronDown, ChevronRight, Pencil, Trash2, Bot } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { MarkdownEditor } from '@/components/ui/MarkdownEditor'
import { AgentDefForm } from '@/components/workflow/AgentDefForm'
import { updateAgentDef, deleteAgentDef } from '@/api/agentDefs'
import { useProjectStore } from '@/stores/projectStore'
import type { AgentDef, AgentDefUpdateRequest } from '@/types/workflow'

export function AgentDefCard({
  def,
  workflowId,
}: {
  def: AgentDef
  workflowId: string
}) {
  const [editing, setEditing] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const queryClient = useQueryClient()
  const currentProject = useProjectStore((s) => s.currentProject)

  const agentDefsKey = ['agent-defs', currentProject, workflowId] as const

  const updateMutation = useMutation({
    mutationFn: (data: AgentDefUpdateRequest) =>
      updateAgentDef(workflowId, def.id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentDefsKey })
      setEditing(false)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: () => deleteAgentDef(workflowId, def.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentDefsKey })
    },
  })

  if (editing) {
    return (
      <AgentDefForm
        initial={def}
        isCreate={false}
        onSubmit={(data) => updateMutation.mutate(data as AgentDefUpdateRequest)}
        onCancel={() => setEditing(false)}
      />
    )
  }

  return (
    <div className="border border-border rounded-lg p-3 hover:bg-muted/20 transition-colors">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <Bot className="h-4 w-4 text-muted-foreground" />
          <span className="font-medium text-sm">{def.id}</span>
          <Badge variant="secondary" className="text-xs">
            {def.model}
          </Badge>
          <span className="text-xs text-muted-foreground">{def.timeout}m timeout</span>
          {def.restart_threshold != null && (
            <span className="text-xs text-muted-foreground">{def.restart_threshold}% restart</span>
          )}
        </div>
        <div className="flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={() => setExpanded(!expanded)}
            title={expanded ? 'Collapse prompt' : 'Expand prompt'}
          >
            {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0"
            onClick={() => setEditing(true)}
            title="Edit"
          >
            <Pencil className="h-3.5 w-3.5" />
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="h-7 w-7 p-0 text-destructive hover:text-destructive"
            onClick={() => {
              if (confirm(`Delete agent definition '${def.id}'?`)) {
                deleteMutation.mutate()
              }
            }}
            title="Delete"
          >
            <Trash2 className="h-3.5 w-3.5" />
          </Button>
        </div>
      </div>
      {!expanded && def.prompt && (
        <p className="text-xs text-muted-foreground mt-1 truncate max-w-xl">
          {def.prompt.split('\n')[0]}
        </p>
      )}
      {expanded && def.prompt && (
        <div className="mt-2">
          <MarkdownEditor
            value={def.prompt}
            readOnly
            minHeight="100px"
            maxHeight="384px"
          />
        </div>
      )}
    </div>
  )
}
