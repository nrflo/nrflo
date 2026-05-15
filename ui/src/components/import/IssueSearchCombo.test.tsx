import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, act, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { IssueSearchCombo } from './IssueSearchCombo'
import { NotConfiguredError } from '@/api/specImport'

type Item = { id: string; label: string }

function renderCombo(overrides: {
  search?: (q: string) => Promise<Item[]>
  onSelect?: (r: Item) => void
  notConfigured?: { missing: string[]; settingsHref: string }
  onNotConfigured?: (missing: string[]) => void
} = {}) {
  const search = overrides.search ?? vi.fn().mockResolvedValue([])
  const onSelect = overrides.onSelect ?? vi.fn()
  render(
    <MemoryRouter>
      <IssueSearchCombo<Item>
        placeholder="Search…"
        search={search}
        renderItem={(r) => <span>{r.label}</span>}
        onSelect={onSelect}
        notConfigured={overrides.notConfigured}
        onNotConfigured={overrides.onNotConfigured}
      />
    </MemoryRouter>
  )
  return { search, onSelect }
}

describe('IssueSearchCombo', () => {
  beforeEach(() => { vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('does not call search before debounce fires', () => {
    const { search } = renderCombo()
    const input = screen.getByPlaceholderText('Search…')

    fireEvent.change(input, { target: { value: 'ab' } })
    act(() => { vi.advanceTimersByTime(400) })

    expect(search).not.toHaveBeenCalled()
  })

  it('calls search with trimmed query after debounce', async () => {
    const { search } = renderCombo()
    const input = screen.getByPlaceholderText('Search…')

    fireEvent.change(input, { target: { value: 'ab' } })
    act(() => { vi.advanceTimersByTime(400) })
    expect(search).not.toHaveBeenCalled()

    await act(async () => { await vi.runAllTimersAsync() })

    expect(search).toHaveBeenCalledWith('ab')
    expect(search).toHaveBeenCalledTimes(1)
  })

  it('does not call search for a single character', async () => {
    const { search } = renderCombo()
    const input = screen.getByPlaceholderText('Search…')

    fireEvent.change(input, { target: { value: 'a' } })
    await act(async () => { await vi.runAllTimersAsync() })

    expect(search).not.toHaveBeenCalled()
  })

  it('debounce resets on each input change', async () => {
    const { search } = renderCombo()
    const input = screen.getByPlaceholderText('Search…')

    fireEvent.change(input, { target: { value: 'ab' } })
    act(() => { vi.advanceTimersByTime(200) })

    // New input before debounce fires — timer resets
    fireEvent.change(input, { target: { value: 'abc' } })
    act(() => { vi.advanceTimersByTime(200) })
    expect(search).not.toHaveBeenCalled()

    await act(async () => { await vi.runAllTimersAsync() })
    expect(search).toHaveBeenCalledTimes(1)
    expect(search).toHaveBeenCalledWith('abc')
  })

  it('calls onSelect with item and closes dropdown on click', async () => {
    const item: Item = { id: '1', label: 'Fix the bug' }
    const search = vi.fn().mockResolvedValue([item])
    const onSelect = vi.fn()

    renderCombo({ search, onSelect })
    const input = screen.getByPlaceholderText('Search…')

    fireEvent.change(input, { target: { value: 'fix' } })
    await act(async () => { await vi.runAllTimersAsync() })

    const result = screen.getByText('Fix the bug')
    fireEvent.click(result)

    expect(onSelect).toHaveBeenCalledWith(item)
    expect(screen.queryByText('Fix the bug')).not.toBeInTheDocument()
  })

  it('calls onNotConfigured when search throws NotConfiguredError', async () => {
    const onNotConfigured = vi.fn()
    const search = vi.fn().mockRejectedValue(new NotConfiguredError(['GITHUB_TOKEN']))

    renderCombo({ search, onNotConfigured })
    const input = screen.getByPlaceholderText('Search…')

    fireEvent.change(input, { target: { value: 'ab' } })
    await act(async () => { await vi.runAllTimersAsync() })

    expect(onNotConfigured).toHaveBeenCalledWith(['GITHUB_TOKEN'])
  })

  it('renders inline amber row with missing vars and settings link', () => {
    renderCombo({
      notConfigured: {
        missing: ['JIRA_BASE_URL', 'JIRA_EMAIL', 'JIRA_API_TOKEN'],
        settingsHref: '/settings?tab=projects&project=p#env-vars',
      },
    })

    expect(screen.getByText(/JIRA_BASE_URL/)).toBeInTheDocument()
    expect(screen.getByText(/JIRA_EMAIL/)).toBeInTheDocument()
    expect(screen.getByText(/JIRA_API_TOKEN/)).toBeInTheDocument()

    const link = screen.getByRole('link', { name: /configure in project settings/i })
    expect(link).toHaveAttribute('href', '/settings?tab=projects&project=p#env-vars')
  })
})
