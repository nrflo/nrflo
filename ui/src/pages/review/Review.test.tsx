import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import { ReviewPage } from './Review'
import type { ReviewItem } from '@/types/review'

vi.mock('@/hooks/useReview', () => ({
  useReviewItems: vi.fn(),
}))

import { useReviewItems } from '@/hooks/useReview'

function makeItem(overrides: Partial<ReviewItem> = {}): ReviewItem {
  return {
    id: 'review-1',
    tool_name: 'my-tool',
    status: 'pending',
    input: {},
    output: {},
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

function setupMocks(data: ReviewItem[] = [], extra = {}) {
  vi.mocked(useReviewItems).mockReturnValue({
    data,
    isLoading: false,
    error: null,
    ...extra,
  } as unknown as ReturnType<typeof useReviewItems>)
}

function renderPage(search = '?status=pending') {
  return render(
    <MemoryRouter initialEntries={[`/review${search}`]}>
      <Routes>
        <Route path="/review" element={<ReviewPage />} />
        <Route path="/review/:id" element={<div>Detail Page</div>} />
      </Routes>
    </MemoryRouter>
  )
}

beforeEach(() => vi.clearAllMocks())

describe('ReviewPage', () => {
  describe('list rendering', () => {
    it('renders review items with tool_name and status badge', () => {
      setupMocks([
        makeItem({ id: 'r1', tool_name: 'tool-alpha', status: 'pending' }),
        makeItem({ id: 'r2', tool_name: 'tool-beta', status: 'approved' }),
      ])
      renderPage()
      expect(screen.getByText('tool-alpha')).toBeInTheDocument()
      expect(screen.getByText('tool-beta')).toBeInTheDocument()
      expect(screen.getByText('pending')).toBeInTheDocument()
      expect(screen.getByText('approved')).toBeInTheDocument()
    })

    it('shows empty state when no items', () => {
      setupMocks([])
      renderPage()
      expect(screen.getByText('No review items.')).toBeInTheDocument()
    })

    it('shows loading state', () => {
      setupMocks([], { isLoading: true, data: undefined })
      renderPage()
      expect(screen.getByText('Loading…')).toBeInTheDocument()
    })

    it('shows error message on fetch failure', () => {
      setupMocks([], { error: new Error('Network error'), data: undefined })
      renderPage()
      expect(screen.getByText('Network error')).toBeInTheDocument()
    })
  })

  describe('status tabs', () => {
    it('renders all four status tabs', () => {
      setupMocks([])
      renderPage()
      expect(screen.getByRole('button', { name: 'Pending' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Approved' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'Rejected' })).toBeInTheDocument()
      expect(screen.getByRole('button', { name: 'All' })).toBeInTheDocument()
    })

    it('clicking Approved tab causes hook to be called with approved status', async () => {
      const user = userEvent.setup()
      setupMocks([])
      renderPage('?status=pending')
      await user.click(screen.getByRole('button', { name: 'Approved' }))
      expect(vi.mocked(useReviewItems)).toHaveBeenCalledWith('approved')
    })

    it('clicking All tab passes undefined status to useReviewItems', async () => {
      const user = userEvent.setup()
      setupMocks([])
      renderPage('?status=pending')
      await user.click(screen.getByRole('button', { name: 'All' }))
      expect(vi.mocked(useReviewItems)).toHaveBeenCalledWith(undefined)
    })
  })

  describe('navigation', () => {
    it('row click navigates to review detail page', async () => {
      const user = userEvent.setup()
      setupMocks([makeItem({ id: 'r-nav', tool_name: 'nav-tool' })])
      renderPage()
      await user.click(screen.getByText('nav-tool'))
      expect(screen.getByText('Detail Page')).toBeInTheDocument()
    })
  })
})
