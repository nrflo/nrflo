import { useState, useMemo } from 'react'
import { ArrowLeft, MessageSquare, Loader2, CheckCircle, XCircle, Cpu, Timer, Terminal, FileText, Tag, Layers, Paperclip, Files, Gauge } from 'lucide-react'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { ResultIcon } from '@/components/ui/ResultIcon'
import { Spinner } from '@/components/ui/Spinner'
import { Tooltip } from '@/components/ui/Tooltip'
import { RenderedMarkdown } from '@/components/ui/RenderedMarkdown'
import { FindingsPanel } from './FindingsPanel'
import { AllFindingsPanel } from './AllFindingsPanel'
import { ArtifactsPanel } from './ArtifactsPanel'
import { AllArtifactsPanel } from './AllArtifactsPanel'
import { ContextLedgerPanel } from './ContextLedgerPanel'
import { HandoffDigestSection } from './HandoffDigestSection'
import { normalizeApiMessages } from './normalizeApiMessages'
import { MessageTable } from './MessageTable'
import { cn } from '@/lib/utils'
import { useSessionMessages } from '@/hooks/useTickets'
import { useSessionPrompt } from '@/hooks/useSessionPrompt'
import type { MessageCategory, WorkflowFindings } from '@/types/workflow'
import type { LucideIcon } from 'lucide-react'
import type { SelectedAgentData } from './PhaseGraph/types'

type DetailTab = 'messages' | 'context' | 'ledger' | 'findings' | 'all-findings' | 'artifacts' | 'all-artifacts'
const DETAIL_TABS: { value: DetailTab; label: string; icon: LucideIcon }[] = [
  { value: 'messages', label: 'Messages', icon: MessageSquare },
  { value: 'context', label: 'Context', icon: FileText },
  { value: 'ledger', label: 'Ledger', icon: Gauge },
  { value: 'findings', label: 'Findings', icon: Tag },
  { value: 'all-findings', label: 'All Findings', icon: Layers },
  { value: 'artifacts', label: 'Artifacts', icon: Paperclip },
  { value: 'all-artifacts', label: 'All Artifacts', icon: Files },
]


function formatDuration(durationSec?: number): string {
  if (!durationSec) return '0s'
  const mins = Math.floor(durationSec / 60)
  const secs = durationSec % 60
  if (mins > 0) return secs > 0 ? `${mins}m ${secs}s` : `${mins}m`
  return `${secs}s`
}

export { formatTime } from './MessageTable'

interface AgentLogDetailProps {
  selectedAgent: SelectedAgentData
  onBack?: () => void
  onResumeSession?: (sessionId: string) => void
  resumePending?: boolean
  agentFindings?: WorkflowFindings
  projectFindings?: Record<string, unknown>
  phaseLayers?: Record<string, number>
  workflowFindings?: Record<string, unknown>
  hideHeader?: boolean
  agentExecutionMode?: 'cli_interactive' | 'api'
  workflowInstanceId?: string
}

export function AgentLogDetail({ selectedAgent, onBack, onResumeSession, resumePending, agentFindings, projectFindings, phaseLayers, workflowFindings, hideHeader, agentExecutionMode, workflowInstanceId }: AgentLogDetailProps) {
  const { agent, historyEntry, session, phaseName } = selectedAgent
  const isInteractive = session?.status === 'user_interactive'
  const isRunning = session?.status === 'running' && !isInteractive
  const result = isRunning || isInteractive ? undefined : (agent?.result || historyEntry?.result || session?.result)
  const modelId = agent?.model_id || historyEntry?.model_id
  const modelName = modelId
    ? modelId.split('-').slice(-2).join('-') || modelId
    : historyEntry?.agent_type || 'agent'
  const duration = historyEntry?.duration_sec ? formatDuration(historyEntry.duration_sec) : null

  const [activeTab, setActiveTab] = useState<DetailTab>('messages')
  const [categoryFilter, setCategoryFilter] = useState<MessageCategory | 'all'>('all')
  const sessionId = session?.id || agent?.session_id || historyEntry?.session_id
  const { data: messagesData, isLoading: messagesLoading } = useSessionMessages(sessionId)
  const { data: promptData, isLoading: promptLoading } = useSessionPrompt(sessionId, activeTab === 'context')

  const messages = messagesData?.messages ?? []

  const normalized = useMemo(() => normalizeApiMessages(messages), [messages])

  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: normalized.length, text: 0, tool: 0, subagent: 0, skill: 0, user_input: 0, error: 0, result: 0, validation: 0, thinking: 0, system_notice: 0, task_notification: 0 }
    for (const m of normalized) {
      const cat = m.category || 'text'
      counts[cat] = (counts[cat] || 0) + 1
    }
    return counts
  }, [normalized])

  const filteredMessages = useMemo(
    () => categoryFilter === 'all' ? normalized : normalized.filter(m => (m.category || 'text') === categoryFilter),
    [normalized, categoryFilter],
  )

  return (
    <div className="flex flex-col h-full">
      {/* Header with back button and agent info */}
      {!hideHeader && (
        <div className="flex items-center gap-2 px-3 py-2 border-b border-border shrink-0">
          {onBack && (
            <button
              onClick={onBack}
              className="flex items-center justify-center w-6 h-6 rounded hover:bg-muted transition-colors shrink-0"
            >
              <ArrowLeft className="h-3.5 w-3.5" />
            </button>
          )}
          <div className={cn(
            'flex items-center justify-center w-7 h-7 rounded-full shrink-0',
            isInteractive && 'bg-blue-100 dark:bg-blue-900/30',
            isRunning && 'bg-yellow-100 dark:bg-yellow-900/30',
            result === 'pass' && 'bg-green-100 dark:bg-green-900/30',
            result === 'fail' && 'bg-red-100 dark:bg-red-900/30',
            !result && !isRunning && !isInteractive && 'bg-gray-100 dark:bg-gray-800'
          )}>
            {isInteractive && <Terminal className="h-3.5 w-3.5 text-blue-600 dark:text-blue-400" />}
            {isRunning && <Loader2 className="h-3.5 w-3.5 text-yellow-600 dark:text-yellow-400 animate-spin" />}
            {result === 'pass' && <CheckCircle className="h-3.5 w-3.5 text-green-600 dark:text-green-400" />}
            {result === 'fail' && <XCircle className="h-3.5 w-3.5 text-red-600 dark:text-red-400" />}
            {!result && !isRunning && !isInteractive && <Cpu className="h-3.5 w-3.5 text-muted-foreground" />}
          </div>
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium truncate">{phaseName.replace(/_/g, ' ')}</div>
            <div className="flex items-center gap-1.5 text-xs text-muted-foreground">
              <span>{modelName}</span>
              {isInteractive && (
                <>
                  <span>·</span>
                  <Badge className="text-[10px] px-1 py-0 bg-blue-100 text-blue-700 dark:bg-blue-900/30 dark:text-blue-400">
                    User controlling
                  </Badge>
                </>
              )}
              {duration && (
                <>
                  <span>·</span>
                  <Timer className="h-3 w-3" />
                  <span>{duration}</span>
                </>
              )}
              {result && (
                <>
                  <span>·</span>
                  <ResultIcon result={result} className="text-[10px]" />
                </>
              )}
            </div>
          </div>
          {onResumeSession && sessionId && (result === 'pass' || result === 'fail') && modelId?.startsWith('claude:') && session?.status !== 'user_interactive' && agentExecutionMode !== 'api' && (
            <Tooltip text="Resume this session in an interactive terminal" placement="top">
              <Button
                variant="outline"
                size="sm"
                onClick={() => onResumeSession(sessionId)}
                disabled={resumePending}
                className="text-blue-600 hover:text-blue-700 shrink-0"
              >
                {resumePending ? (
                  <Spinner size="sm" className="mr-1.5" />
                ) : (
                  <Terminal className="h-3.5 w-3.5 mr-1.5" />
                )}
                Resume
              </Button>
            </Tooltip>
          )}
        </div>
      )}

      {/* Top-level tab bar */}
      <div className="flex items-center gap-1 px-3 py-1 border-b border-border shrink-0">
        {DETAIL_TABS.map(({ value, label, icon: Icon }) => (
          <button
            key={value}
            onClick={() => setActiveTab(value)}
            className={cn(
              'px-2.5 py-1 text-xs font-medium rounded transition-colors',
              activeTab === value
                ? 'bg-muted text-foreground'
                : 'text-muted-foreground hover:text-foreground hover:bg-muted/50',
            )}
          >
            <Icon className="h-3 w-3 inline-block mr-1 -mt-0.5" />
            {label}
          </button>
        ))}
      </div>

      {/* Content area */}
      <div className="flex-1 overflow-y-auto overflow-x-hidden px-3 py-2">
        {activeTab === 'all-artifacts' ? (
          <AllArtifactsPanel workflowInstanceId={workflowInstanceId} />
        ) : activeTab === 'artifacts' ? (
          <ArtifactsPanel workflowInstanceId={workflowInstanceId} sessionId={sessionId} />
        ) : activeTab === 'all-findings' ? (
          <AllFindingsPanel
            workflowFindings={workflowFindings}
            agentFindings={agentFindings}
            projectFindings={projectFindings}
            phaseLayers={phaseLayers}
          />
        ) : activeTab === 'findings' ? (
          <FindingsPanel
            projectFindings={projectFindings}
            agentFindings={agentFindings}
            selectedAgentType={selectedAgent.agent?.agent_type || selectedAgent.historyEntry?.agent_type || null}
          />
        ) : activeTab === 'ledger' ? (
          <div className="space-y-4">
            <HandoffDigestSection sessionId={sessionId} enabled={activeTab === 'ledger'} />
            <ContextLedgerPanel sessionId={sessionId} enabled={activeTab === 'ledger'} />
          </div>
        ) : activeTab === 'context' ? (
          promptLoading ? (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <Loader2 className="h-6 w-6 mb-2 animate-spin opacity-50" />
              <p className="text-xs">Loading prompt context...</p>
            </div>
          ) : (promptData?.prompt || promptData?.system_prompt) ? (
            <div className="space-y-4">
              {promptData.prompt && (
                <div>
                  <p className="text-xs text-muted-foreground mb-1">User prompt</p>
                  <RenderedMarkdown content={promptData.prompt} />
                </div>
              )}
              {promptData.system_prompt && (
                <div>
                  <p className="text-xs text-muted-foreground mb-1">System prompt suffix</p>
                  <RenderedMarkdown content={promptData.system_prompt} />
                </div>
              )}
            </div>
          ) : (
            <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
              <FileText className="h-8 w-8 mb-2 opacity-30" />
              <p className="text-xs">No prompt context available</p>
            </div>
          )
        ) : messagesLoading && normalized.length === 0 ? (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <Loader2 className="h-6 w-6 mb-2 animate-spin opacity-50" />
            <p className="text-xs">Loading messages...</p>
          </div>
        ) : normalized.length > 0 ? (
          <MessageTable
            filteredMessages={filteredMessages}
            categoryFilter={categoryFilter}
            setCategoryFilter={setCategoryFilter}
            categoryCounts={categoryCounts}
            normalizedCount={normalized.length}
          />
        ) : (
          <div className="flex flex-col items-center justify-center py-8 text-muted-foreground">
            <MessageSquare className="h-8 w-8 mb-2 opacity-30" />
            <p className="text-xs">No messages available</p>
          </div>
        )}
      </div>
    </div>
  )
}
