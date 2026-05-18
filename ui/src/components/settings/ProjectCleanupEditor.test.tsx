import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { ProjectCleanupEditor, type CleanupFormState } from './ProjectCleanupEditor'
import { renderWithQuery } from '@/test/utils'

const PROJECT_ID = 'proj-1'

beforeEach(() => vi.clearAllMocks())

const defaultCleanup: CleanupFormState = { enabled: false, retentionLimit: 0 }

describe('ProjectCleanupEditor', () => {
  it('value enabled=false hides retention input and shows indefinite text', () => {
    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} value={defaultCleanup} onChange={vi.fn()} />)
    const toggle = screen.getByRole('switch', { name: /enable cleanup/i })
    expect(toggle).toHaveAttribute('aria-checked', 'false')
    expect(screen.queryByPlaceholderText('e.g. 1000')).not.toBeInTheDocument()
    expect(screen.getByText(/kept indefinitely/i)).toBeInTheDocument()
  })

  it('toggling calls onChange with enabled=true', async () => {
    const onChange = vi.fn()
    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} value={defaultCleanup} onChange={onChange} />)

    const user = userEvent.setup()
    await user.click(screen.getByRole('switch', { name: /enable cleanup/i }))

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ enabled: true }))
  })

  it('changing retention limit calls onChange with numeric value', async () => {
    const onChange = vi.fn()
    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} value={{ enabled: true, retentionLimit: 100 }} onChange={onChange} />)

    const user = userEvent.setup()
    const retentionInput = screen.getByPlaceholderText('e.g. 1000')
    await user.clear(retentionInput)
    await user.type(retentionInput, '1000')

    expect(onChange).toHaveBeenLastCalledWith(expect.objectContaining({ retentionLimit: 1000 }))
  })

  it('server error is rendered verbatim', () => {
    renderWithQuery(
      <ProjectCleanupEditor
        projectId={PROJECT_ID}
        value={defaultCleanup}
        onChange={vi.fn()}
        serverError="retention_limit must be positive"
      />
    )
    expect(screen.getByText('retention_limit must be positive')).toBeInTheDocument()
  })

  it('value enabled=true shows retention input and hides indefinite text', () => {
    renderWithQuery(<ProjectCleanupEditor projectId={PROJECT_ID} value={{ enabled: true, retentionLimit: 50 }} onChange={vi.fn()} />)
    expect(screen.getByRole('switch', { name: /enable cleanup/i })).toHaveAttribute('aria-checked', 'true')
    expect(screen.getByPlaceholderText('e.g. 1000')).toBeInTheDocument()
    expect(screen.queryByText(/kept indefinitely/i)).not.toBeInTheDocument()
  })
})
