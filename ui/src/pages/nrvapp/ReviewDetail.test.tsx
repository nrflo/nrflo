import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ReviewDetailPage } from './ReviewDetail'
import type { NrvappReviewItem } from '@/types/nrvapp'

vi.mock('@/hooks/useNrvapp', () => ({
  useReviewItem: vi.fn(),
  useUpdateReviewDraft: vi.fn(),
  useApproveReview: vi.fn(),
  useRejectReview: vi.fn(),
}))

vi.mock('@/components/nrvapp/DiffPreview', () => ({
  DiffPreview: () => <div data-testid="diff-preview" />,
}))

vi.mock('@/components/ui/MarkdownEditor', () => ({
  MarkdownEditor: ({
    value,
    onChange,
    readOnly,
  }: {
    value: string
    onChange?: (v: string) => void
    readOnly?: boolean
  }) => (
    <textarea
      data-testid={readOnly ? 'markdown-readonly' : 'markdown-editor'}
      defaultValue={value}
      onChange={(e) => onChange?.(e.target.value)}
      readOnly={readOnly}
    />
  ),
}))

import {
  useReviewItem,
  useUpdateReviewDraft,
  useApproveReview,
  useRejectReview,
} from '@/hooks/useNrvapp'

function makeItem(overrides: Partial<NrvappReviewItem> = {}): NrvappReviewItem {
  return {
    id: 'review-1',
    tool_name: 'my-tool',
    status: 'pending',
    input: { key: 'value' },
    output: { result: 'output' },
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

const mockUpdateMutate = vi.fn()
const mockApproveMutate = vi.fn()
const mockRejectMutate = vi.fn()

function setupMocks(item: NrvappReviewItem | null = makeItem(), extra = {}) {
  vi.mocked(useReviewItem).mockReturnValue({
    data: item ?? undefined,
    isLoading: false,
    error: item === null ? new Error('Not found') : null,
    ...extra,
  } as unknown as ReturnType<typeof useReviewItem>)
  vi.mocked(useUpdateReviewDraft).mockReturnValue({
    mutate: mockUpdateMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useUpdateReviewDraft>)
  vi.mocked(useApproveReview).mockReturnValue({
    mutate: mockApproveMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useApproveReview>)
  vi.mocked(useRejectReview).mockReturnValue({
    mutate: mockRejectMutate,
    isPending: false,
  } as unknown as ReturnType<typeof useRejectReview>)
}

function renderPage(id = 'review-1') {
  return render(
    <MemoryRouter initialEntries={[`/nrvapp/review/${id}`]}>
      <Routes>
        <Route path="/nrvapp/review/:id" element={<ReviewDetailPage />} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('ReviewDetailPage', () => {
  describe('loading/error states', () => {
    it('shows loading state', () => {
      setupMocks(null, { isLoading: true, error: null, data: undefined })
      renderPage()
      expect(screen.getByText('Loading…')).toBeInTheDocument()
    })

    it('shows error state when item not found', () => {
      setupMocks(null, { isLoading: false, error: new Error('fail'), data: undefined })
      renderPage()
      expect(screen.getByText('Error loading review item.')).toBeInTheDocument()
    })
  })

  describe('item display', () => {
    it('renders tool_name and status badge', () => {
      setupMocks(makeItem({ tool_name: 'test-tool', status: 'pending' }))
      renderPage()
      expect(screen.getByText('test-tool')).toBeInTheDocument()
      expect(screen.getByText('pending')).toBeInTheDocument()
    })

    it('renders Approve and Reject buttons for pending item', () => {
      setupMocks(makeItem({ status: 'pending' }))
      renderPage()
      expect(screen.getByRole('button', { name: 'Approve' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Reject' })).toBeInTheDocument()
    })

    it('does not render action buttons for approved item', () => {
      setupMocks(makeItem({ status: 'approved' }))
      renderPage()
      expect(screen.queryByRole('button', { name: 'Approve' })).not.toBeInTheDocument()
      expect(screen.queryByRole('button', { name: 'Reject' })).not.toBeInTheDocument()
    })
  })

  describe('approve flow', () => {
    it('Approve click calls approveMutation.mutate with item id', async () => {
      const user = userEvent.setup()
      setupMocks(makeItem({ id: 'item-abc', status: 'pending' }))
      renderPage('item-abc')
      await user.click(screen.getByRole('button', { name: 'Approve' }))
      expect(mockApproveMutate).toHaveBeenCalledWith('item-abc')
    })

    it('Approve button disabled when isPending', () => {
      setupMocks(makeItem({ status: 'pending' }))
      vi.mocked(useApproveReview).mockReturnValue({
        mutate: mockApproveMutate,
        isPending: true,
      } as unknown as ReturnType<typeof useApproveReview>)
      renderPage()
      expect(screen.getByRole('button', { name: 'Approve' })).toBeDisabled()
    })
  })

  describe('reject flow', () => {
    it('Reject click opens reject modal', async () => {
      const user = userEvent.setup()
      setupMocks(makeItem({ status: 'pending' }))
      renderPage()
      await user.click(screen.getByRole('button', { name: 'Reject' }))
      expect(screen.getByText('Reject Review')).toBeInTheDocument()
    })

    it('submitting reject modal calls rejectMutation.mutate with id and reason', async () => {
      const user = userEvent.setup()
      setupMocks(makeItem({ id: 'item-xyz', status: 'pending' }))
      renderPage('item-xyz')
      // Open dialog (clicks the main-page Reject button, first in DOM)
      const [mainRejectBtn] = screen.getAllByRole('button', { name: 'Reject' })
      await user.click(mainRejectBtn)
      await user.type(screen.getByPlaceholderText('Reason for rejection…'), 'bad output')
      // Submit button is the last Reject button in DOM (dialog footer)
      const rejectButtons = screen.getAllByRole('button', { name: /^Reject$/ })
      await user.click(rejectButtons[rejectButtons.length - 1])
      expect(mockRejectMutate).toHaveBeenCalledWith(
        { id: 'item-xyz', reason: 'bad output' },
        expect.any(Object)
      )
    })

    it('Reject submit button disabled when reason is empty', async () => {
      const user = userEvent.setup()
      setupMocks(makeItem({ status: 'pending' }))
      renderPage()
      const [mainRejectBtn] = screen.getAllByRole('button', { name: 'Reject' })
      await user.click(mainRejectBtn)
      // Dialog's Reject button is the last one
      const rejectButtons = screen.getAllByRole('button', { name: /^Reject$/ })
      expect(rejectButtons[rejectButtons.length - 1]).toBeDisabled()
    })

    it('dialog reject button disabled while isPending', async () => {
      const user = userEvent.setup()
      setupMocks(makeItem({ status: 'pending' }))
      vi.mocked(useRejectReview).mockReturnValue({
        mutate: mockRejectMutate,
        isPending: true,
      } as unknown as ReturnType<typeof useRejectReview>)
      renderPage()
      const [mainRejectBtn] = screen.getAllByRole('button', { name: 'Reject' })
      await user.click(mainRejectBtn)
      await user.type(screen.getByPlaceholderText('Reason for rejection…'), 'reason')
      const rejectButtons = screen.getAllByRole('button', { name: /^Reject$/ })
      expect(rejectButtons[rejectButtons.length - 1]).toBeDisabled()
    })
  })

  describe('save draft', () => {
    it('Save draft button calls updateDraft.mutate with id and parsed JSON', async () => {
      const user = userEvent.setup()
      setupMocks(makeItem({ id: 'item-draft', output: { result: 'original' } }))
      renderPage('item-draft')
      await user.click(screen.getByRole('button', { name: 'Save draft' }))
      expect(mockUpdateMutate).toHaveBeenCalledWith(
        expect.objectContaining({ id: 'item-draft' })
      )
    })

    it('Save draft button disabled while isPending', () => {
      setupMocks(makeItem())
      vi.mocked(useUpdateReviewDraft).mockReturnValue({
        mutate: mockUpdateMutate,
        isPending: true,
      } as unknown as ReturnType<typeof useUpdateReviewDraft>)
      renderPage()
      expect(screen.getByRole('button', { name: 'Save draft' })).toBeDisabled()
    })
  })
})
