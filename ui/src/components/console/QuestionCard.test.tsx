import { describe, it, expect, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { renderWithQuery } from '@/test/utils'
import { ApprovalCard } from './ApprovalCard'
import { composeAnswer, parseQuestions } from './QuestionCard'
import * as useConsoleChats from '@/hooks/useConsoleChats'
import type { PendingApproval } from '@/types/consoleChat'

vi.mock('@/hooks/useConsoleChats', async () => {
  const actual = await vi.importActual<typeof import('@/hooks/useConsoleChats')>('@/hooks/useConsoleChats')
  return { ...actual, useReplyApproval: vi.fn() }
})

const questionInput = JSON.stringify({
  questions: [
    {
      question: 'How do you want to fix it?',
      header: 'Hook fix',
      options: [
        { label: 'Patch the hook', description: 'resolve the cd prefix' },
        { label: 'Manual commit', description: 'you run it yourself' },
      ],
    },
  ],
})

const questionApproval: PendingApproval = {
  approval_id: 'q1',
  kind: 'PreToolUse',
  tool: 'AskUserQuestion',
  command: '[AskUserQuestion]',
  cwd: '/tmp',
  reason: '',
  input: questionInput,
}

function mockMutation() {
  const mutate = vi.fn()
  vi.mocked(useConsoleChats.useReplyApproval).mockReturnValue({
    mutate,
    isPending: false,
  } as unknown as ReturnType<typeof useConsoleChats.useReplyApproval>)
  return mutate
}

describe('QuestionCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('renders an AskUserQuestion approval as a question card and answers with the picked option', async () => {
    const mutate = mockMutation()
    const user = userEvent.setup()
    renderWithQuery(<ApprovalCard sid="sid-1" approval={questionApproval} />)

    expect(screen.getByTestId('question-card')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Allow' })).not.toBeInTheDocument()

    const answerButton = screen.getByRole('button', { name: 'Answer' })
    expect(answerButton).toBeDisabled()

    await user.click(screen.getByRole('button', { name: 'Manual commit' }))
    await user.click(answerButton)

    expect(mutate).toHaveBeenCalledWith({
      sid: 'sid-1',
      aid: 'q1',
      decision: 'answer',
      answer: 'Manual commit',
    })
  })

  it('a typed custom answer wins over a picked option', async () => {
    const mutate = mockMutation()
    const user = userEvent.setup()
    renderWithQuery(<ApprovalCard sid="sid-1" approval={questionApproval} />)

    await user.type(screen.getByPlaceholderText('Custom answer…'), 'patch it but keep blocked repos')
    await user.click(screen.getByRole('button', { name: 'Answer' }))

    expect(mutate).toHaveBeenCalledWith({
      sid: 'sid-1',
      aid: 'q1',
      decision: 'answer',
      answer: 'patch it but keep blocked repos',
    })
  })

  it('falls back to the generic approval card on an unparseable payload', () => {
    mockMutation()
    renderWithQuery(
      <ApprovalCard sid="sid-1" approval={{ ...questionApproval, input: 'not json' }} />
    )
    expect(screen.queryByTestId('question-card')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Allow' })).toBeInTheDocument()
  })

  it('renders the answered terminal state from the resolved push', () => {
    mockMutation()
    renderWithQuery(
      <ApprovalCard
        sid="sid-1"
        approval={questionApproval}
        resolved={{ approval_id: 'q1', decision: 'answer', reason: 'Manual commit' }}
      />
    )
    expect(screen.getByText('Answered: Manual commit')).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Answer' })).not.toBeInTheDocument()
  })
})

describe('parseQuestions/composeAnswer', () => {
  it('parses the questions array and rejects junk', () => {
    expect(parseQuestions(questionInput)).toHaveLength(1)
    expect(parseQuestions('nope')).toBeNull()
    expect(parseQuestions('{"questions":[]}')).toBeNull()
    expect(parseQuestions(undefined)).toBeNull()
  })

  it('labels multi-question answers with headers', () => {
    const questions = [
      { question: 'Fix?', header: 'Hook fix' },
      { question: 'Push too?', header: '' },
    ]
    expect(composeAnswer(questions, ['patch', 'yes'])).toBe('Hook fix: patch; Push too?: yes')
    expect(composeAnswer(questions, ['patch'])).toBe('patch')
  })
})
