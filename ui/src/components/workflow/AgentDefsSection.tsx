import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Badge } from '@/components/ui/Badge'
import { Toggle } from '@/components/ui/Toggle'
import { AgentDefForm } from '@/components/workflow/AgentDefForm'
import { AgentDefCard } from '@/components/workflow/AgentDefCard'
import { LayerPolicyControl } from '@/components/workflow/LayerPolicyControl'
import { listAgentDefs, createAgentDef } from '@/api/agentDefs'
import { listLayerPolicies, setLayerPolicy } from '@/api/workflowLayerPolicies'
import { useProjectStore } from '@/stores/projectStore'
import type { AgentDefCreateRequest } from '@/types/workflow'

export function AgentDefsSection({ workflowId, groups, project }: { workflowId: string; groups: string[]; project?: string }) {
  const [creating, setCreating] = useState(false)
  const queryClient = useQueryClient()
  const currentProject = useProjectStore((s) => s.currentProject)
  const scope = project ?? currentProject

  const agentDefsQueryKey = ['agent-defs', scope, workflowId] as const
  const layerPoliciesQueryKey = ['workflow-layer-policies', scope, workflowId] as const

  const { data: defs, isLoading } = useQuery({
    queryKey: agentDefsQueryKey,
    queryFn: () => listAgentDefs(workflowId, project),
  })

  const { data: layerPolicies } = useQuery({
    queryKey: layerPoliciesQueryKey,
    queryFn: () => listLayerPolicies(workflowId),
  })

  const createMutation = useMutation({
    mutationFn: (data: AgentDefCreateRequest) =>
      createAgentDef(workflowId, data, project),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: agentDefsQueryKey })
      setCreating(false)
    },
  })

  const pauseToggleMutation = useMutation({
    mutationFn: ({ layer, passPolicy, pauseAfter }: { layer: number; passPolicy: string; pauseAfter: boolean }) =>
      setLayerPolicy(workflowId, layer, passPolicy, pauseAfter),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: layerPoliciesQueryKey })
    },
  })

  // Split consultants from phase defs; consultants are excluded from layer layout
  const consultantDefs = defs?.filter((d) => d.consultant) ?? []
  const phaseDefs = defs?.filter((d) => !d.consultant) ?? []

  // Group phase defs by layer, sorted ascending
  const byLayer: Record<number, typeof defs> = {}
  for (const def of phaseDefs) {
    const l = def.layer ?? 0
    if (!byLayer[l]) byLayer[l] = []
    byLayer[l]!.push(def)
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
          submitError={createMutation.error?.message}
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
          const policy = layerPolicies?.layer_policies?.[layer]
          const pauseAfter = layerPolicies?.layer_pause?.[layer] ?? false
          return (
            <div key={layer} className="space-y-2">
              <div className="flex items-center gap-2 flex-wrap">
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
                <Toggle
                  checked={pauseAfter}
                  onChange={(checked) =>
                    pauseToggleMutation.mutate({ layer, passPolicy: policy ?? 'any', pauseAfter: checked })
                  }
                  label="Pause after"
                />
              </div>
              <div className="space-y-2">
                {layerDefs.map((def) => (
                  <AgentDefCard
                    key={def.id}
                    def={def}
                    workflowId={workflowId}
                    groups={groups}
                    project={project}
                  />
                ))}
              </div>
            </div>
          )
        })}
      </div>

      {consultantDefs.length > 0 && (
        <div className="space-y-2 mt-4">
          <h4 className="text-xs font-semibold text-muted-foreground uppercase tracking-wider">
            Consultants
          </h4>
          <div className="space-y-2">
            {consultantDefs.map((def) => (
              <AgentDefCard
                key={def.id}
                def={def}
                workflowId={workflowId}
                groups={groups}
              />
            ))}
          </div>
        </div>
      )}
    </div>
  )
}
