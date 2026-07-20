import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { HandoffDigestSection } from './HandoffDigestSection'
import * as useSessionHandoffDigestHook from '@/hooks/useSessionHandoffDigest'
import type { HandoffDigest, HandoffDigestEvent } from '@/types/handoffDigest'

vi.mock('@/hooks/useSessionHandoffDigest', () => ({
  useSessionHandoffDigest: vi.fn(),
}))

function digest(overrides: Partial<HandoffDigest> = {}): HandoffDigest {
  return {
    content: 'Digest body text',
    version: 1,
    fold_count: 3,
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function mockDigest(overrides: Partial<ReturnType<typeof useSessionHandoffDigestHook.useSessionHandoffDigest>> = {}) {
  vi.mocked(useSessionHandoffDigestHook.useSessionHandoffDigest).mockReturnValue({
    data: undefined,
    isLoading: false,
    live: undefined,
    ...overrides,
  } as ReturnType<typeof useSessionHandoffDigestHook.useSessionHandoffDigest>)
}

describe('HandoffDigestSection', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders fold count and timestamp from query data, and content when expanded', async () => {
    const user = userEvent.setup()
    mockDigest({ data: digest({ content: 'Hello from digest', fold_count: 3 }) })
    renderWithQuery(<HandoffDigestSection sessionId="s1" enabled />)

    expect(screen.getByText('Handoff digest')).toBeInTheDocument()
    expect(screen.getByText('3 folds')).toBeInTheDocument()
    expect(screen.queryByText('Hello from digest')).not.toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Expand handoff digest' }))
    expect(screen.getByText('Hello from digest')).toBeInTheDocument()
  })

  it('prefers the live overlay over query data and updates on re-render', () => {
    mockDigest({ data: digest({ fold_count: 3 }) })
    const { rerender } = renderWithQuery(<HandoffDigestSection sessionId="s1" enabled />)
    expect(screen.getByText('3 folds')).toBeInTheDocument()

    const live: HandoffDigestEvent = { ...digest({ fold_count: 7 }), session_id: 's1' }
    mockDigest({ data: digest({ fold_count: 3 }), live })
    rerender(<HandoffDigestSection sessionId="s1" enabled />)

    expect(screen.getByText('7 folds')).toBeInTheDocument()
    expect(screen.queryByText('3 folds')).not.toBeInTheDocument()
  })

  it('renders nothing when there is no digest and no live overlay', () => {
    mockDigest({ data: null })
    const { container } = renderWithQuery(<HandoffDigestSection sessionId="s1" enabled />)
    expect(container).toBeEmptyDOMElement()
  })
})
