import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { PlanReviseDialog } from './PlanReviseDialog'
import { ApiError } from '@/api/client'
import type { PlanQuestion } from '@/types/plan'

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

vi.mock('@/hooks/usePlan', () => ({
  useRevisePlan: vi.fn(),
}))

import { toast } from 'sonner'
import { useRevisePlan } from '@/hooks/usePlan'

const questions: PlanQuestion[] = [
  { id: 'q1', question: 'Which database?' },
  { id: 'q2', question: 'Which auth provider?' },
]

describe('PlanReviseDialog', () => {
  const reviseMutate = vi.fn()
  const onClose = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    vi.mocked(useRevisePlan).mockReturnValue({ mutate: reviseMutate, isPending: false } as any)
  })

  it('renders one answer input per open question', () => {
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} questions={questions} />)

    expect(screen.getByText('Which database?')).toBeInTheDocument()
    expect(screen.getByText('Which auth provider?')).toBeInTheDocument()
    expect(screen.getAllByPlaceholderText('Your answer')).toHaveLength(2)
  })

  it('renders no open-questions section when there are none', () => {
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} />)
    expect(screen.queryByText('Open questions')).not.toBeInTheDocument()
  })

  it('submits {revision, feedback, answers} pinned to the given revision', async () => {
    const user = userEvent.setup()
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} questions={questions} />)

    await user.type(screen.getByPlaceholderText(/what should the planner change/i), 'Use postgres')
    const answerInputs = screen.getAllByPlaceholderText('Your answer')
    await user.type(answerInputs[0], 'Postgres')
    await user.click(screen.getByRole('button', { name: /submit revision/i }))

    expect(reviseMutate).toHaveBeenCalledWith(
      {
        instanceId: 'inst-1',
        params: {
          revision: 3,
          feedback: 'Use postgres',
          answers: [{ question_id: 'q1', answer: 'Postgres' }],
        },
      },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) })
    )
  })

  it('omits feedback and answers when both are left blank', async () => {
    const user = userEvent.setup()
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={5} />)

    await user.click(screen.getByRole('button', { name: /submit revision/i }))

    expect(reviseMutate).toHaveBeenCalledWith(
      { instanceId: 'inst-1', params: { revision: 5, feedback: undefined, answers: undefined } },
      expect.anything()
    )
  })

  it('calls onClose on successful submission', () => {
    reviseMutate.mockImplementation((_vars, opts) => opts.onSuccess())
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} />)

    screen.getByRole('button', { name: /submit revision/i }).click()
    expect(onClose).toHaveBeenCalled()
  })

  it('disables submit and cancel while a revision is pending', () => {
    vi.mocked(useRevisePlan).mockReturnValue({ mutate: reviseMutate, isPending: true } as any)
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} />)

    expect(screen.getByRole('button', { name: /submit revision/i })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('a 409 error surfaces the stale-revision toast, not a generic error', () => {
    reviseMutate.mockImplementation((_vars, opts) => opts.onError(new ApiError(409, 'revision mismatch')))
    render(<PlanReviseDialog onClose={onClose} instanceId="inst-1" revision={3} />)

    screen.getByRole('button', { name: /submit revision/i }).click()

    expect(toast.error).toHaveBeenCalledWith(
      'The plan changed since this page loaded — reload to see the latest revision.'
    )
    expect(toast.error).not.toHaveBeenCalledWith(expect.stringContaining('Failed to revise plan'))
  })
})
