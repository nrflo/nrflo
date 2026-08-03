import { SessionFlowGraph } from './SessionFlowGraph'
import { SessionToolDistribution } from './SessionToolDistribution'
import { SessionCostRollup } from './SessionCostRollup'
import { useSessionFlow, useSessionStats } from '@/hooks/useSessionFlow'

export function SessionDetail({ sessionId }: { sessionId: string }) {
  const { data: flow, isLoading: flowLoading } = useSessionFlow(sessionId)
  const { data: stats, isLoading: statsLoading } = useSessionStats(sessionId)

  const isEmpty =
    !flowLoading &&
    !statsLoading &&
    (!flow || flow.nodes.length === 0) &&
    (!stats?.tool_calls || stats.tool_calls.length === 0) &&
    !stats?.subtree_cost_usd &&
    !stats?.subtree_tokens

  return (
    <div className="border-t border-border pt-4 space-y-4">
      <h2 className="text-sm font-medium text-muted-foreground">
        Session {sessionId.substring(0, 8)}
      </h2>

      {flowLoading || statsLoading ? (
        <div className="text-sm text-muted-foreground">Loading...</div>
      ) : isEmpty ? (
        <div className="text-sm text-muted-foreground">
          No downstream activity recorded for this session.
        </div>
      ) : (
        <>
          <SessionFlowGraph flow={flow} />
          <SessionToolDistribution toolCalls={stats?.tool_calls} />
          <SessionCostRollup stats={stats} />
        </>
      )}
    </div>
  )
}
