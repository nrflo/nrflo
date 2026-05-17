export function MergedFromBadge({ data }: { data: Record<string, unknown> }) {
  const agentIds = (data.agentIds as string[]) ?? []
  return (
    <div className="px-2 py-1 bg-blue-50 border border-blue-200 rounded-full text-xs text-blue-700 whitespace-nowrap pointer-events-none">
      merged: {agentIds.join(', ')}
    </div>
  )
}
