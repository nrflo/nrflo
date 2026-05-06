import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Dropdown } from '@/components/ui/Dropdown'
import { Input } from '@/components/ui/Input'
import { Button } from '@/components/ui/Button'
import { setLayerPolicy, deleteLayerPolicy } from '@/api/workflowLayerPolicies'
import type { LayerPassPolicy } from '@/types/workflow'

type PolicyKind = 'any' | 'all' | 'quorum' | 'percent'

const POLICY_OPTIONS = [
  { value: 'any', label: 'any (1 agent must pass)' },
  { value: 'all', label: 'all (every agent must pass)' },
  { value: 'quorum', label: 'quorum (N agents must pass)' },
  { value: 'percent', label: 'percent (P% must pass)' },
]

function parsePolicy(policy: LayerPassPolicy | undefined): { kind: PolicyKind; num: string } {
  if (!policy || policy === 'any') return { kind: 'any', num: '' }
  if (policy === 'all') return { kind: 'all', num: '' }
  if (policy.startsWith('quorum:')) return { kind: 'quorum', num: policy.slice(7) }
  if (policy.startsWith('percent:')) return { kind: 'percent', num: policy.slice(8) }
  return { kind: 'any', num: '' }
}

function buildPolicy(kind: PolicyKind, num: string): LayerPassPolicy {
  if (kind === 'quorum') return `quorum:${num}`
  if (kind === 'percent') return `percent:${num}`
  return kind
}

interface Props {
  workflowId: string
  layer: number
  agentCount: number
  current?: LayerPassPolicy
  layerPoliciesQueryKey: readonly unknown[]
}

export function LayerPolicyControl({ workflowId, layer, agentCount, current, layerPoliciesQueryKey }: Props) {
  const queryClient = useQueryClient()

  const parsed = parsePolicy(current)
  const [kind, setKind] = useState<PolicyKind>(parsed.kind)
  const [num, setNum] = useState(parsed.num)
  const [error, setError] = useState<string | null>(null)

  // Reset local draft when external value changes (e.g. WS update)
  const extParsed = parsePolicy(current)
  const [lastCurrent, setLastCurrent] = useState(current)
  if (current !== lastCurrent) {
    setLastCurrent(current)
    setKind(extParsed.kind)
    setNum(extParsed.num)
    setError(null)
  }

  const mutation = useMutation({
    mutationFn: (policy: LayerPassPolicy) =>
      policy === 'any'
        ? deleteLayerPolicy(workflowId, layer)
        : setLayerPolicy(workflowId, layer, policy),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: layerPoliciesQueryKey })
    },
  })

  function validate(): string | null {
    if (kind === 'quorum') {
      const n = parseInt(num, 10)
      if (isNaN(n) || n < 1 || n > agentCount) {
        return `N must be between 1 and ${agentCount}`
      }
    }
    if (kind === 'percent') {
      const p = parseInt(num, 10)
      if (isNaN(p) || p < 1 || p > 100) {
        return 'P must be between 1 and 100'
      }
    }
    return null
  }

  function handleSave() {
    const validationError = validate()
    if (validationError) {
      setError(validationError)
      return
    }
    setError(null)
    const policy = buildPolicy(kind, num)
    mutation.mutate(policy)
  }

  function handleKindChange(v: string) {
    setKind(v as PolicyKind)
    setNum('')
    setError(null)
  }

  const isDirty = buildPolicy(kind, num) !== (current ?? 'any')

  return (
    <div className="flex items-center gap-2 flex-wrap">
      <span className="text-xs text-muted-foreground font-medium shrink-0">
        Layer {layer} policy:
      </span>
      <div className="w-52">
        <Dropdown
          value={kind}
          onChange={handleKindChange}
          options={POLICY_OPTIONS}
          disabled={mutation.isPending}
        />
      </div>
      {(kind === 'quorum' || kind === 'percent') && (
        <div className="w-20">
          <Input
            type="number"
            value={num}
            onChange={(e) => { setNum(e.target.value); setError(null) }}
            placeholder={kind === 'quorum' ? 'N' : 'P%'}
            min={1}
            max={kind === 'quorum' ? agentCount : 100}
            disabled={mutation.isPending}
            className="h-9 text-sm"
          />
        </div>
      )}
      {error && <span className="text-xs text-destructive">{error}</span>}
      {isDirty && (
        <Button
          size="sm"
          variant="outline"
          className="h-7 text-xs"
          onClick={handleSave}
          disabled={mutation.isPending}
        >
          Save
        </Button>
      )}
    </div>
  )
}
