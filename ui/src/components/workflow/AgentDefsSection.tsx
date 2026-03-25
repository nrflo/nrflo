import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { AgentDefForm } from '@/components/workflow/AgentDefForm'
import { AgentDefCard } from '@/components/workflow/AgentDefCard'
import { listAgentDefs, createAgentDef } from '@/api/agentDefs'
import { useProjectStore } from '@/stores/projectStore'
import type { AgentDefCreateRequest } from '@/types/workflow'

export function AgentDefsSection({ workflowId, groups }: { workflowId: string; groups: string[] }) {
  const [creating, setCreating] = useState(false)
  const queryClient = useQueryClient()
  const currentProject = useProjectStore((s) => s.currentProject)

  const queryKey = ['agent-defs', currentProject, workflowId] as const

  const { data: defs, isLoading } = useQuery({
    queryKey,
    queryFn: () => listAgentDefs(workflowId),
  })

  const createMutation = useMutation({
    mutationFn: (data: AgentDefCreateRequest) =>
      createAgentDef(workflowId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      setCreating(false)
    },
  })

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          Agent Definitions
        </h4>
        <Button variant="ghost" size="sm" className="h-7" onClick={() => setCreating(!creating)}>
          <Plus className="h-3.5 w-3.5 mr-1" />
          Add Agent
        </Button>
      </div>

      {creating && (
        <AgentDefForm
          isCreate
          groups={groups}
          onSubmit={(data) => createMutation.mutate(data as AgentDefCreateRequest)}
          onCancel={() => setCreating(false)}
        />
      )}

      {isLoading && <p className="text-xs text-muted-foreground">Loading...</p>}

      {defs && defs.length === 0 && !creating && (
        <p className="text-xs text-muted-foreground italic">No agent definitions yet.</p>
      )}

      <div className="space-y-2">
        {defs?.map((def) => (
          <AgentDefCard
            key={def.id}
            def={def}
            workflowId={workflowId}
            groups={groups}
          />
        ))}
      </div>
    </div>
  )
}
