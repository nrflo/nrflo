import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ApprovalCard } from './ApprovalCard'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import type { PendingApproval } from '@/types/consoleChat'
import type { ResolvedApproval } from './chatStream'

vi.mock('@/hooks/useConsoleChats', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useConsoleChats')>('@/hooks/useConsoleChats')
  return { ...actual, useReplyApproval: vi.fn() }
})

const approval: PendingApproval = {
  approval_id: 'a1',
  kind: 'bash',
  command: 'rm -rf /tmp/x',
  cwd: '/tmp',
  reason: 'destructive command',
}

function mockMutation(overrides: Partial<ReturnType<typeof useConsoleChats.useReplyApproval>> = {}) {
  const mutate = vi.fn()
  vi.mocked(useConsoleChats.useReplyApproval).mockReturnValue({
    mutate,
    isPending: false,
    ...overrides,
  } as ReturnType<typeof useConsoleChats.useReplyApproval>)
  return mutate
}

describe('ApprovalCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('calls the mutation with allow when Allow is clicked', async () => {
    const mutate = mockMutation()
    const user = userEvent.setup()
    renderWithQuery(<ApprovalCard sid="sid-1" approval={approval} />)

    await user.click(screen.getByRole('button', { name: 'Allow' }))

    expect(mutate).toHaveBeenCalledWith({ sid: 'sid-1', aid: 'a1', decision: 'allow' })
  })

  it('calls the mutation with deny when Deny is clicked', async () => {
    const mutate = mockMutation()
    const user = userEvent.setup()
    renderWithQuery(<ApprovalCard sid="sid-1" approval={approval} />)

    await user.click(screen.getByRole('button', { name: 'Deny' }))

    expect(mutate).toHaveBeenCalledWith({ sid: 'sid-1', aid: 'a1', decision: 'deny' })
  })

  it('renders the deny-on-timeout state from a console_chat.approval_resolved push', () => {
    mockMutation()
    const resolved: ResolvedApproval = { approval_id: 'a1', decision: 'deny', reason: 'nrflo: approval timed out' }
    renderWithQuery(<ApprovalCard sid="sid-1" approval={approval} resolved={resolved} />)

    expect(screen.getByText('Denied — timed out')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Allow' })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Deny' })).not.toBeInTheDocument()
  })

  it('a resolved card disables its buttons (allow path shows Allowed, no buttons rendered)', () => {
    mockMutation()
    const resolved: ResolvedApproval = { approval_id: 'a1', decision: 'allow' }
    renderWithQuery(<ApprovalCard sid="sid-1" approval={approval} resolved={resolved} />)

    expect(screen.getByText('Allowed')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('a plain deny (not timed out) renders as Denied', () => {
    mockMutation()
    const resolved: ResolvedApproval = { approval_id: 'a1', decision: 'deny', reason: 'user rejected' }
    renderWithQuery(<ApprovalCard sid="sid-1" approval={approval} resolved={resolved} />)

    expect(screen.getByText('Denied')).toBeInTheDocument()
  })

  it('renders identically whether the pending approval came from reload or from a WS push', () => {
    mockMutation()
    const { unmount } = renderWithQuery(<ApprovalCard sid="sid-1" approval={approval} />)
    const fromReload = screen.getByTestId('approval-card').innerHTML
    unmount()

    // Same shape as what useConsoleChatStream would produce from a live
    // console_chat.approval_request push — the component takes no signal
    // about origin, so identical props must render identical output.
    mockMutation()
    renderWithQuery(<ApprovalCard sid="sid-1" approval={{ ...approval }} />)
    const fromWS = screen.getByTestId('approval-card').innerHTML

    expect(fromWS).toBe(fromReload)
  })
})
