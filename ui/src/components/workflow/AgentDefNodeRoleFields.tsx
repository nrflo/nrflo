import { Dropdown } from '@/components/ui/Dropdown'

type NodeRole = 'static' | 'planner' | 'fanout_template'

interface AgentDefNodeRoleFieldsProps {
  nodeRole: NodeRole
  setNodeRole: (v: NodeRole) => void
  description: string
  setDescription: (v: string) => void
}

const NODE_ROLE_OPTIONS = [
  { value: 'static', label: 'Static (runs as a workflow phase)' },
  { value: 'planner', label: 'Planner (drafts plan manifests)' },
  { value: 'fanout_template', label: 'Fanout template (bindable by plan nodes)' },
]

export function AgentDefNodeRoleFields({ nodeRole, setNodeRole, description, setDescription }: AgentDefNodeRoleFieldsProps) {
  const descriptionRequired = nodeRole === 'fanout_template'
  return (
    <div className="space-y-3">
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">Node role</label>
        <Dropdown value={nodeRole} onChange={(v) => setNodeRole(v as NodeRole)} options={NODE_ROLE_OPTIONS} />
        <p className="text-xs text-muted-foreground mt-1">Non-static defs never auto-execute as a workflow phase.</p>
      </div>
      <div>
        <label className="block text-xs font-medium text-muted-foreground mb-1">
          Description {descriptionRequired && <span className="text-destructive">*</span>}
        </label>
        <input
          type="text"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          placeholder="When to use this agent (tool-description quality)"
          className="w-full rounded-md border border-border bg-background px-3 py-1.5 text-sm"
        />
        {descriptionRequired && (
          <p className="text-xs text-muted-foreground mt-1">Required for fanout templates — shown to the planner as selection text.</p>
        )}
      </div>
    </div>
  )
}
