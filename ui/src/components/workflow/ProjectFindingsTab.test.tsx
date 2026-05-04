import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { ProjectFindingsTab } from './ProjectFindingsTab'

vi.mock('sonner', () => ({ toast: { error: vi.fn() } }))

vi.mock('@/hooks/useTickets', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useTickets')>('@/hooks/useTickets')
  return {
    ...actual,
    useProjectFindings: vi.fn(),
    useUpsertProjectFinding: vi.fn(),
    useDeleteProjectFinding: vi.fn(),
  }
})

function renderTab(projectId = 'test-project') {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={qc}>
      <ProjectFindingsTab projectId={projectId} />
    </QueryClientProvider>
  )
}

describe('ProjectFindingsTab', () => {
  let useProjectFindings: any
  let useUpsertProjectFinding: any
  let useDeleteProjectFinding: any
  let mockUpsertMutate: ReturnType<typeof vi.fn>
  let mockDeleteMutate: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    const hooks = await import('@/hooks/useTickets')
    useProjectFindings = hooks.useProjectFindings as any
    useUpsertProjectFinding = hooks.useUpsertProjectFinding as any
    useDeleteProjectFinding = hooks.useDeleteProjectFinding as any

    vi.clearAllMocks()

    mockUpsertMutate = vi.fn()
    mockDeleteMutate = vi.fn()

    useProjectFindings.mockReturnValue({ data: {} })
    useUpsertProjectFinding.mockReturnValue({ mutate: mockUpsertMutate, isPending: false })
    useDeleteProjectFinding.mockReturnValue({ mutate: mockDeleteMutate, isPending: false })
  })

  describe('empty state', () => {
    it('shows empty state when findings is empty object', () => {
      useProjectFindings.mockReturnValue({ data: {} })
      renderTab()
      expect(screen.getByText('No project findings yet.')).toBeInTheDocument()
    })

    it('shows empty state when data is undefined', () => {
      useProjectFindings.mockReturnValue({ data: undefined })
      renderTab()
      expect(screen.getByText('No project findings yet.')).toBeInTheDocument()
    })
  })

  describe('filtering', () => {
    beforeEach(() => {
      useProjectFindings.mockReturnValue({
        data: { foo: 'bar', _internal: 'hidden', baz: { nested: 1 } },
      })
    })

    it('shows visible keys and hides internal keys by default', () => {
      renderTab()
      expect(screen.getByText('foo')).toBeInTheDocument()
      expect(screen.getByText('baz')).toBeInTheDocument()
      expect(screen.queryByText('_internal')).not.toBeInTheDocument()
    })

    it('shows internal keys after toggling "Show internal keys"', async () => {
      const user = userEvent.setup()
      renderTab()
      expect(screen.queryByText('_internal')).not.toBeInTheDocument()
      await user.click(screen.getByRole('switch'))
      expect(await screen.findByText('_internal')).toBeInTheDocument()
    })

    it('hides internal keys again when toggle is turned off', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByRole('switch'))
      expect(await screen.findByText('_internal')).toBeInTheDocument()
      await user.click(screen.getByRole('switch'))
      expect(screen.queryByText('_internal')).not.toBeInTheDocument()
    })
  })

  describe('Add Finding dialog', () => {
    it('opens dialog when Add Finding is clicked', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByRole('button', { name: /add finding/i }))
      // Dialog opened — Key and Value form fields are visible
      expect(screen.getByPlaceholderText('finding-key')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('Value...')).toBeInTheDocument()
    })

    it('calls upsert mutation with projectId, key, value on submit', async () => {
      const user = userEvent.setup()
      renderTab('proj-123')
      await user.click(screen.getByRole('button', { name: /add finding/i }))
      await user.type(screen.getByPlaceholderText('finding-key'), 'my-key')
      await user.type(screen.getByPlaceholderText('Value...'), 'my-value')
      await user.click(screen.getByRole('button', { name: 'Add' }))
      expect(mockUpsertMutate).toHaveBeenCalledWith(
        { projectId: 'proj-123', key: 'my-key', value: 'my-value' },
        expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) })
      )
    })

    it('submit button disabled when key is empty', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByRole('button', { name: /add finding/i }))
      const addBtn = screen.getByRole('button', { name: 'Add' })
      expect(addBtn).toBeDisabled()
    })

    it('submit button disabled when isPending is true', async () => {
      useUpsertProjectFinding.mockReturnValue({ mutate: mockUpsertMutate, isPending: true })
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByRole('button', { name: /add finding/i }))
      await user.type(screen.getByPlaceholderText('finding-key'), 'key')
      expect(screen.getByRole('button', { name: 'Add' })).toBeDisabled()
    })

    it('does not call mutate when key is whitespace only', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByRole('button', { name: /add finding/i }))
      await user.type(screen.getByPlaceholderText('finding-key'), '   ')
      await user.click(screen.getByRole('button', { name: 'Add' }))
      expect(mockUpsertMutate).not.toHaveBeenCalled()
    })
  })

  describe('Edit Finding dialog', () => {
    beforeEach(() => {
      useProjectFindings.mockReturnValue({ data: { foo: 'original-value' } })
    })

    it('opens dialog with pre-filled key and value on edit', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByTitle('Edit'))
      expect(screen.getByText('Edit Finding')).toBeInTheDocument()
      expect(screen.getByPlaceholderText('finding-key')).toHaveValue('foo')
      expect(screen.getByPlaceholderText('Value...')).toHaveValue('original-value')
    })

    it('key input is disabled in edit mode', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByTitle('Edit'))
      expect(screen.getByPlaceholderText('finding-key')).toBeDisabled()
    })

    it('calls upsert with same key and updated value on save', async () => {
      const user = userEvent.setup()
      renderTab('proj-456')
      await user.click(screen.getByTitle('Edit'))
      const textarea = screen.getByPlaceholderText('Value...')
      await user.clear(textarea)
      await user.type(textarea, 'new-value')
      await user.click(screen.getByRole('button', { name: 'Save' }))
      expect(mockUpsertMutate).toHaveBeenCalledWith(
        { projectId: 'proj-456', key: 'foo', value: 'new-value' },
        expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) })
      )
    })
  })

  describe('Delete Finding', () => {
    beforeEach(() => {
      useProjectFindings.mockReturnValue({ data: { foo: 'bar' } })
    })

    it('opens ConfirmDialog when delete button is clicked', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByTitle('Delete'))
      expect(screen.getByText('Delete Finding')).toBeInTheDocument()
      expect(screen.getByText(/Are you sure you want to delete the finding "foo"/)).toBeInTheDocument()
    })

    it('calls delete mutation with projectId and key on confirm', async () => {
      const user = userEvent.setup()
      renderTab('proj-789')
      await user.click(screen.getByTitle('Delete'))
      // ConfirmDialog has a "Delete" button with visible text
      await user.click(screen.getByText('Delete'))
      expect(mockDeleteMutate).toHaveBeenCalledWith(
        { projectId: 'proj-789', key: 'foo' },
        expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) })
      )
    })

    it('does NOT call delete mutation when cancel is clicked', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByTitle('Delete'))
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(mockDeleteMutate).not.toHaveBeenCalled()
    })

    it('closes ConfirmDialog on cancel without mutation', async () => {
      const user = userEvent.setup()
      renderTab()
      await user.click(screen.getByTitle('Delete'))
      expect(screen.getByText('Delete Finding')).toBeInTheDocument()
      await user.click(screen.getByRole('button', { name: 'Cancel' }))
      expect(screen.queryByText('Delete Finding')).not.toBeInTheDocument()
    })
  })
})
