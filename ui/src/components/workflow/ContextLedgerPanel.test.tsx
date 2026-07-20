import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import { renderWithQuery } from '@/test/utils'
import { ContextLedgerPanel } from './ContextLedgerPanel'
import * as useSessionContextLedgerHook from '@/hooks/useSessionContextLedger'
import type { ContextLedgerSnapshot, LedgerEntry } from '@/types/contextLedger'

vi.mock('@/hooks/useSessionContextLedger', () => ({
  useSessionContextLedger: vi.fn(),
}))

function entry(overrides: Partial<LedgerEntry> = {}): LedgerEntry {
  return {
    id: 'e1',
    kind: 'dialog',
    tokens_est: 100,
    born_turn: 1,
    last_ref_turn: 1,
    source: 'user',
    superseded: false,
    approx: false,
    ...overrides,
  }
}

function snapshot(overrides: Partial<ContextLedgerSnapshot> = {}): ContextLedgerSnapshot {
  return {
    session_id: 's1',
    entries: [entry()],
    totals_by_kind: { dialog: 100 },
    ...overrides,
  }
}

function mockLedger(overrides: Partial<ReturnType<typeof useSessionContextLedgerHook.useSessionContextLedger>> = {}) {
  vi.mocked(useSessionContextLedgerHook.useSessionContextLedger).mockReturnValue({
    data: undefined,
    isLoading: false,
    liveTotals: undefined,
    ...overrides,
  } as ReturnType<typeof useSessionContextLedgerHook.useSessionContextLedger>)
}

describe('ContextLedgerPanel', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows a loading state while the query is in flight', () => {
    mockLedger({ isLoading: true })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)
    expect(screen.getByText('Loading context ledger...')).toBeInTheDocument()
  })

  it('shows an empty state when there is no snapshot', () => {
    mockLedger({ data: null })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)
    expect(screen.getByText('No context ledger available')).toBeInTheDocument()
  })

  it('renders per-kind breakdown rows and totals from the snapshot', () => {
    mockLedger({
      data: snapshot({
        entries: [entry({ id: 'e1', kind: 'dialog', tokens_est: 100 }), entry({ id: 'e2', kind: 'tool_use', tokens_est: 50 })],
        totals_by_kind: { dialog: 100, tool_use: 50 },
      }),
    })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)

    expect(screen.getByText('Dialog')).toBeInTheDocument()
    expect(screen.getByText('100 tok')).toBeInTheDocument()
    expect(screen.getByText('Tool use')).toBeInTheDocument()
    expect(screen.getByText('50 tok')).toBeInTheDocument()
    expect(screen.getByText('150 total tokens')).toBeInTheDocument()
    expect(screen.getByText('2 entries')).toBeInTheDocument()
  })

  it('prefers live totals over the snapshot when both are present', () => {
    mockLedger({
      data: snapshot({ totals_by_kind: { dialog: 100 } }),
      liveTotals: { session_id: 's1', total_tokens: 250, entry_count: 5, totals_by_kind: { dialog: 250 } },
    })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)

    expect(screen.getByText('250 total tokens')).toBeInTheDocument()
    expect(screen.getByText('5 entries')).toBeInTheDocument()
  })

  it('renders superseded entries in an excluded section', () => {
    mockLedger({
      data: snapshot({
        entries: [
          entry({ id: 'e1', kind: 'dialog', tokens_est: 100, superseded: false }),
          entry({ id: 'e2', kind: 'tool_result', tokens_est: 20, source: 'stale-read', superseded: true }),
        ],
        totals_by_kind: { dialog: 100 },
      }),
    })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)

    expect(screen.getByText('Superseded — excluded from totals (1)')).toBeInTheDocument()
    expect(screen.getByText('Tool result · stale-read')).toBeInTheDocument()
  })

  it('does not render a superseded section when nothing is superseded', () => {
    mockLedger({ data: snapshot() })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)
    expect(screen.queryByText(/Superseded/)).not.toBeInTheDocument()
  })

  it('renders the budget bar when a positive budget is supplied', () => {
    mockLedger({ data: snapshot({ totals_by_kind: { dialog: 100 } }) })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled budgetTokens={1000} />)
    expect(screen.getByText('Budget')).toBeInTheDocument()
    expect(screen.getByText('100 / 1000 tok')).toBeInTheDocument()
  })

  it('omits the budget bar when no budget is supplied', () => {
    mockLedger({ data: snapshot() })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled />)
    expect(screen.queryByText('Budget')).not.toBeInTheDocument()
  })

  it('omits the budget bar when budget is zero or negative', () => {
    mockLedger({ data: snapshot() })
    renderWithQuery(<ContextLedgerPanel sessionId="s1" enabled budgetTokens={0} />)
    expect(screen.queryByText('Budget')).not.toBeInTheDocument()
  })
})
