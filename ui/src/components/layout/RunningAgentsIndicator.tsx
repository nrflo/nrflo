import { useState, useRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { Link, useNavigate } from 'react-router-dom'
import { Spinner } from '@/components/ui/Spinner'
import { useRunningAgents } from '@/hooks/useRunningAgents'
import { useProjectStore } from '@/stores/projectStore'
import type { RunningAgent } from '@/types/agents'

function formatElapsed(seconds: number): string {
  if (seconds < 60) return `${Math.floor(seconds)}s`
  if (seconds < 3600) {
    const m = Math.floor(seconds / 60)
    const s = Math.floor(seconds % 60)
    return s > 0 ? `${m}m ${s}s` : `${m}m`
  }
  const h = Math.floor(seconds / 3600)
  const m = Math.floor((seconds % 3600) / 60)
  return m > 0 ? `${h}h ${m}m` : `${h}h`
}

function formatAgentLabel(agent: RunningAgent): string {
  if (agent.agent_type === 'conflict-resolver' && agent.workflow_id === '_conflict_resolution') {
    return 'Merge Conflict Resolver'
  }
  return `${agent.workflow_id} / ${agent.agent_type}`
}

function groupByProject(agents: RunningAgent[]): Map<string, RunningAgent[]> {
  const map = new Map<string, RunningAgent[]>()
  for (const agent of agents) {
    const key = agent.project_name || agent.project_id
    const list = map.get(key) ?? []
    list.push(agent)
    map.set(key, list)
  }
  return map
}

export function RunningAgentsIndicator() {
  const { data } = useRunningAgents()
  const navigate = useNavigate()
  const currentProject = useProjectStore((s) => s.currentProject)
  const [visible, setVisible] = useState(false)
  const [coords, setCoords] = useState({ top: 0, left: 0 })
  const triggerRef = useRef<HTMLDivElement>(null)
  const hideTimeout = useRef<ReturnType<typeof setTimeout> | null>(null)

  const clearHideTimeout = useCallback(() => {
    if (hideTimeout.current) {
      clearTimeout(hideTimeout.current)
      hideTimeout.current = null
    }
  }, [])

  const scheduleHide = useCallback(() => {
    clearHideTimeout()
    hideTimeout.current = setTimeout(() => setVisible(false), 150)
  }, [clearHideTimeout])

  const showPopover = useCallback(() => {
    clearHideTimeout()
    if (!triggerRef.current) return
    const rect = triggerRef.current.getBoundingClientRect()
    setCoords({
      top: rect.bottom + 6,
      left: rect.left + rect.width / 2,
    })
    setVisible(true)
  }, [clearHideTimeout])

  const handleAgentClick = useCallback(
    (agent: RunningAgent) => {
      if (agent.project_id !== currentProject) {
        useProjectStore.getState().setCurrentProject(agent.project_id)
      }
      if (agent.ticket_id) {
        navigate(`/tickets/${agent.ticket_id}?tab=workflow`)
      } else {
        navigate('/project-workflows')
      }
      setVisible(false)
    },
    [currentProject, navigate],
  )

  if (!data || data.count === 0) return null

  const grouped = groupByProject(data.agents)

  return (
    <>
      <div
        ref={triggerRef}
        onMouseEnter={showPopover}
        onMouseLeave={scheduleHide}
        className="relative inline-flex items-center cursor-pointer"
      >
        <Spinner size="sm" />
        <span className="absolute -top-1.5 -right-2.5 flex h-4 min-w-4 items-center justify-center rounded-full bg-primary text-[10px] font-bold text-primary-foreground px-0.5">
          {data.count}
        </span>
      </div>
      {visible &&
        createPortal(
          <div
            onMouseEnter={clearHideTimeout}
            onMouseLeave={scheduleHide}
            className="fixed z-[100] -translate-x-1/2 min-w-64 max-w-fit rounded-lg bg-gray-900 text-white dark:bg-gray-100 dark:text-gray-900 shadow-lg p-3 text-xs"
            style={{ top: coords.top, left: coords.left }}
          >
            <div className="font-semibold mb-2">
              Running Agents ({data.count})
            </div>
            {Array.from(grouped.entries()).map(([projectName, agents]) => (
              <div key={projectName} className="mb-2 last:mb-0">
                <div className="font-medium text-gray-300 dark:text-gray-600 mb-1">
                  {projectName}
                </div>
                {agents.map((agent) => (
                  <Link
                    key={agent.session_id}
                    to={
                      agent.ticket_id
                        ? `/tickets/${agent.ticket_id}?tab=workflow`
                        : '/project-workflows'
                    }
                    onClick={(e) => {
                      e.preventDefault()
                      handleAgentClick(agent)
                    }}
                    className="block pl-3 py-1 rounded hover:bg-white/10 dark:hover:bg-black/10 transition-colors whitespace-nowrap"
                  >
                    <span className="text-gray-200 dark:text-gray-700 mr-1">
                      └
                    </span>
                    {agent.ticket_id && <>{agent.ticket_id} &middot; </>}{formatAgentLabel(agent)}
                    <span className="ml-1 text-gray-400 dark:text-gray-500">
                      ({formatElapsed(agent.elapsed_sec)})
                    </span>
                  </Link>
                ))}
              </div>
            ))}
          </div>,
          document.body,
        )}
    </>
  )
}
