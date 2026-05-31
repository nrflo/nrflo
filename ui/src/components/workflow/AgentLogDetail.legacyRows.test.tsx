import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { AgentLogDetail } from './AgentLogDetail'
import * as ticketsApi from '@/api/tickets'
import type { SelectedAgentData } from './PhaseGraph/types'
import type { ActiveAgentV4, AgentSession } from '@/types/workflow'

Element.prototype.scrollIntoView = vi.fn()

vi.mock('@/api/tickets', async () => {
  const actual = await vi.importActual('@/api/tickets')
  return { ...actual, getSessionMessages: vi.fn() }
})

function makeAgent(): ActiveAgentV4 {
  return {
    agent_id: 'a1', agent_type: 'implementor', phase: 'implementation',
    model_id: 'claude-sonnet-4-5', cli: 'claude', pid: 12345,
    started_at: '2026-01-01T00:00:00Z',
  }
}

function makeSession(overrides: Partial<AgentSession> = {}): AgentSession {
  return {
    id: 'session-1', project_id: 'test-project', ticket_id: 'TICKET-1',
    workflow_instance_id: 'wi-1', phase: 'implementation', workflow: 'feature',
    agent_type: 'implementor', model_id: 'claude-sonnet-4-5', status: 'running',
    message_count: 5, restart_count: 0,
    created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const baseAgent: SelectedAgentData = {
  phaseName: 'implementation',
  agent: makeAgent(),
  session: makeSession(),
}

function renderDetail(selectedAgent: SelectedAgentData = baseAgent) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <AgentLogDetail selectedAgent={selectedAgent} onBack={vi.fn()} />
    </QueryClientProvider>
  )
}

// Helpers for injecting legacy rows that TypeScript doesn't know about
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const legacyMsg = (content: string, category: string) => ({ content, category: category as any, created_at: '2026-01-01T00:00:10Z' })

describe('AgentLogDetail - legacy API-mode row normalization', () => {
  beforeEach(() => { vi.clearAllMocks() })

  it('folds tool_use_start+tool_use_input pair into one Tools tab entry (not two)', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_use:start] id=c1 name=Bash', 'tool_use_start'),
        legacyMsg('[tool_use:input] id=c1 input=git status', 'tool_use_input'),
      ],
      total: 2,
    })
    renderDetail()
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    const tabs = screen.getAllByRole('tab')
    expect(tabs[2].textContent).toContain('1') // Tools: 1 (pair folds to one)
  })

  it('normalizes tool_result into a Tools tab entry (non-hidden tool)', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_result] name=WebFetch output=fetched data', 'tool_result'),
      ],
      total: 1,
    })
    renderDetail()
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    const tabs = screen.getAllByRole('tab')
    expect(tabs[2].textContent).toContain('1') // Tools: 1
    expect(tabs[6].textContent).toContain('0') // Errors: 0
  })

  it('normalizes tool_error into an Errors tab entry (not Tools)', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_result:error] name=Write output=permission denied', 'tool_error'),
      ],
      total: 1,
    })
    renderDetail()
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    const tabs = screen.getAllByRole('tab')
    expect(tabs[6].textContent).toContain('1') // Errors: 1
    expect(tabs[2].textContent).toContain('0') // Tools: 0
  })

  it('tool_error row renders with Error badge in tool column', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_result:error] name=Write output=permission denied', 'tool_error'),
      ],
      total: 1,
    })
    renderDetail()
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows).toHaveLength(1)
    const toolCell = rows[0].querySelectorAll(':scope > td')[1]
    expect(within(toolCell as HTMLElement).getByText('Error')).toBeInTheDocument()
  })

  it('tool_error row has red left-rail destructive styling', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_result:error] name=Bash output=failed', 'tool_error'),
      ],
      total: 1,
    })
    renderDetail()
    await waitFor(() => expect(screen.getByText('1 messages')).toBeInTheDocument())
    const rows = document.querySelectorAll('[data-testid="message-row"]')
    expect(rows[0].className).toContain('border-l-destructive')
  })

  it('mixed legacy and clean rows produce correct tab counts after normalization', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_use:start] id=c1 name=Bash', 'tool_use_start'),
        legacyMsg('[tool_use:input] id=c1 input=ls', 'tool_use_input'),
        { content: 'plain text', category: 'text', created_at: '2026-01-01T00:00:20Z' },
        legacyMsg('[tool_result:error] name=Write output=err', 'tool_error'),
      ],
      total: 4,
    })
    renderDetail()
    // 4 raw rows → 3 normalized (pair folds + 1 text + 1 error)
    await waitFor(() => expect(screen.getByText('3 messages')).toBeInTheDocument())
    const tabs = screen.getAllByRole('tab')
    expect(tabs[0].textContent).toContain('3') // All: 3
    expect(tabs[1].textContent).toContain('1') // Text: 1
    expect(tabs[2].textContent).toContain('1') // Tools: 1
    expect(tabs[6].textContent).toContain('1') // Errors: 1
  })

  it('no tab count label contains raw legacy category strings after normalization', async () => {
    vi.mocked(ticketsApi.getSessionMessages).mockResolvedValue({
      session_id: 'session-1',
      messages: [
        legacyMsg('[tool_use:start] id=c1 name=Bash', 'tool_use_start'),
        legacyMsg('[tool_use:input] id=c1 input=ls', 'tool_use_input'),
        legacyMsg('[tool_result] name=Read output=ok', 'tool_result'),
        legacyMsg('[tool_result:error] name=Write output=err', 'tool_error'),
      ],
      total: 4,
    })
    renderDetail()
    // 4 raw rows → 2 normalized: pair folds to 1 tool, Read result dropped, Write error = 1 error
    await waitFor(() => expect(screen.getByText('2 messages')).toBeInTheDocument())
    const tablist = document.querySelector('[role="tablist"]')!
    const rawCategories = ['tool_use_start', 'tool_use_input', 'tool_result', 'tool_error']
    for (const raw of rawCategories) {
      expect(tablist.textContent).not.toContain(raw)
    }
  })
})
