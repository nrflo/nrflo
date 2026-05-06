import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { AgentDefForm } from '@/components/workflow/AgentDefForm'
import { AgentDefCard } from '@/components/workflow/AgentDefCard'
import { LayerPolicyControl } from '@/components/workflow/LayerPolicyControl'
import { listAgentDefs, createAgentDef } from '@/api/agentDefs'
import { listLayerPolicies } from '@/api/workflowLayerPolicies'
import { useProjectStore } from '@/stores/projectStore'
import type { AgentDefCreateRequest } from '@/types/workflow'

export function AgentDefsSection({ workflowId, groups }: { workflowId: string; groups: string[] }) {
  const [creating, setCreating] = useState(false)
  const queryClient = useQueryClient()
  const currentProject = useProjectStore((s) => s.currentProject)

  const agentDefsQueryKey = ['agent-defs', currentProject, workflowId] as const
  const layerPoliciesQueryKey = ['workflow-layer-policies', currentProject, workflowId] as const

  const { data: defs, isLoading } = useQuery({
    queryKey: agentDefsQueryKey,
    queryFn: () => listAgentDefs(workflowId),
  })

  const { data: layerPolicies } = useQuery({
    queryKey: layerPoliciesQueryKey,
    queryFn: () => listLayerPolicies(workflowId),
  })

  const createMutation = useMutation({
    mutationFn: (data: AgentDefCreateRequest) =>
      createAgentDef(workflowId, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentDefsQueryKey })
      setCreating(false)
    },
  })

  // Group defs by layer, sorted ascending
  const byLayer: Record<number, typeof defs> = {}
  if (defs) {
    for (const def of defs) {
      const l = def.layer ?? 0
      if (!byLayer[l]) byLayer[l] = []
      byLayer[l]!.push(def)
    }
  }
  const sortedLayers = Object.keys(byLayer).map(Number).sort((a, b) => a - b)

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

      <div className="space-y-4">
        {sortedLayers.map((layer) => {
          const layerDefs = byLayer[layer] ?? []
          const isMulti = layerDefs.length >= 2
          const policy = layerPolicies?.[layer]
          return (
            <div key={layer} className="space-y-2">
              <div className="flex items-center gap-2">
                {isMulti ? (
                  <LayerPolicyControl
                    workflowId={workflowId}
                    layer={layer}
                    agentCount={layerDefs.length}
                    current={policy}
                    layerPoliciesQueryKey={layerPoliciesQueryKey}
                  />
                ) : (
                  <div className="flex items-center gap-2">
                    <span className="text-xs text-muted-foreground font-medium">Layer {layer}:</span>
                    <Badge variant="outline" className="text-xs">any</Badge>
                  </div>
                )}
              </div>
              <div className="space-y-2">
                {layerDefs.map((def) => (
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
        })}
      </div>
    </div>
  )
}
