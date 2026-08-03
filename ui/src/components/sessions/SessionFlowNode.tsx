import { Handle, Position } from '@xyflow/react'
import { Link } from 'react-router-dom'
import { Badge } from '@/components/ui/Badge'
import { StatusCell } from '@/components/ui/StatusCell'
import type { SessionFlowNodeData } from './sessionFlowLayout'

// Sub-workflow/dynamic-run nodes carry a workflow_instance_id and link to
// the project workflows page, the existing detail view for that instance.
function detailLink(data: SessionFlowNodeData): { to: string; label: string } | null {
  const { node } = data
  if (node.workflow_instance_id) {
    return { to: '/project-workflows', label: 'View workflow' }
  }
  return null
}

export function SessionFlowNode({ data }: { data: SessionFlowNodeData }) {
  const { node, isRoot } = data
  const link = detailLink(data)

  return (
    <div
      className="rounded-lg border border-border bg-background px-3 py-2 shadow-sm text-xs space-y-1"
      style={{ width: 260 }}
    >
      <Handle type="target" position={Position.Top} className="opacity-0" />
      <div className="flex items-center justify-between gap-2">
        <span className="font-mono truncate" title={node.session_id}>
          {node.session_id.substring(0, 8)}
        </span>
        <div className="flex items-center gap-1 shrink-0">
          {isRoot && <Badge variant="secondary">root</Badge>}
          <Badge variant="outline">{node.kind}</Badge>
        </div>
      </div>
      {node.agent_type && (
        <div className="truncate text-muted-foreground" title={node.agent_type}>
          {node.agent_type}
        </div>
      )}
      <div className="flex items-center justify-between gap-2">
        <StatusCell status={node.status} />
        {node.result && <span className="text-muted-foreground">{node.result}</span>}
      </div>
      {link && (
        <Link to={link.to} className="text-primary hover:underline">
          {link.label}
        </Link>
      )}
      <Handle type="source" position={Position.Bottom} className="opacity-0" />
    </div>
  )
}
