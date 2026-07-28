import { useMemo, useState } from 'react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { useReplyApproval } from '@/hooks/useConsoleChats'
import type { PendingApproval } from '@/types/consoleChat'
import type { ResolvedApproval } from './chatStream'

interface QuestionOption {
  label: string
  description?: string
}

interface ChatQuestion {
  question: string
  header?: string
  multiSelect?: boolean
  options?: QuestionOption[]
}

// parseQuestions extracts the AskUserQuestion payload from the approval's
// verbatim tool-input JSON; null means unparseable (the caller falls back to
// the generic approval card, whose Allow maps to the server-side plain-text
// redirect, so the chat cannot deadlock).
export function parseQuestions(input?: string): ChatQuestion[] | null {
  if (!input) return null
  try {
    const payload = JSON.parse(input) as { questions?: ChatQuestion[] }
    return Array.isArray(payload.questions) && payload.questions.length > 0 ? payload.questions : null
  } catch {
    return null
  }
}

// composeAnswer flattens per-question answers into the single string the
// model receives: bare for one question, 'Header: answer' pairs for several.
export function composeAnswer(questions: ChatQuestion[], answers: string[]): string {
  if (answers.length === 1) return answers[0]
  return answers.map((answer, i) => `${questions[i].header || questions[i].question}: ${answer}`).join('; ')
}

interface QuestionCardProps {
  sid: string
  approval: PendingApproval
  questions: ChatQuestion[]
  resolved?: ResolvedApproval
}

// Interactive card for an AskUserQuestion approval: per-question option
// buttons (multiSelect toggles), an optional free-form answer input, and a
// combined submit that resolves the approval with decision='answer' — the
// server feeds the answer back to the model as the tool feedback, so the
// CLI's own picker never opens (it would be unreachable inside the hidden
// PTY).
export function QuestionCard({ sid, approval, questions, resolved }: QuestionCardProps) {
  const replyMutation = useReplyApproval()
  // Per-question state: selected option labels + free text.
  const [picks, setPicks] = useState<Record<number, string[]>>({})
  const [custom, setCustom] = useState<Record<number, string>>({})

  const answers = useMemo(
    () =>
      questions.map((_q, i) => {
        const text = (custom[i] ?? '').trim()
        if (text) return text
        return (picks[i] ?? []).join(', ')
      }),
    [questions, picks, custom]
  )
  const complete = answers.every((a) => a.length > 0)

  const toggle = (qi: number, label: string, multi: boolean) => {
    setPicks((prev) => {
      const current = prev[qi] ?? []
      if (!multi) return { ...prev, [qi]: current[0] === label ? [] : [label] }
      return {
        ...prev,
        [qi]: current.includes(label) ? current.filter((l) => l !== label) : [...current, label],
      }
    })
  }

  const submit = () => {
    if (!complete) return
    replyMutation.mutate({
      sid,
      aid: approval.approval_id,
      decision: 'answer',
      answer: composeAnswer(questions, answers),
    })
  }

  return (
    <div
      className="rounded-md border border-amber-400/50 bg-amber-50/50 dark:bg-amber-950/20 px-3 py-2 text-xs"
      data-testid="question-card"
    >
      <div className="font-semibold text-foreground">The agent asks</div>
      {questions.map((q, qi) => (
        <div key={qi} className="mt-2">
          <div className="text-foreground/90">
            {q.header && <span className="font-semibold">{q.header} · </span>}
            {q.question}
          </div>
          {resolved ? null : (
            <div className="mt-1 flex flex-wrap gap-1.5">
              {(q.options ?? []).map((o) => {
                const selected = (picks[qi] ?? []).includes(o.label)
                return (
                  <Button
                    key={o.label}
                    size="sm"
                    variant={selected ? 'default' : 'outline'}
                    title={o.description}
                    onClick={() => toggle(qi, o.label, q.multiSelect === true)}
                  >
                    {o.label}
                  </Button>
                )
              })}
              <Input
                className="h-8 w-56 text-xs"
                placeholder="Custom answer…"
                value={custom[qi] ?? ''}
                onChange={(e) => setCustom((prev) => ({ ...prev, [qi]: e.target.value }))}
              />
            </div>
          )}
        </div>
      ))}
      {resolved ? (
        <div className="mt-2 text-xs font-semibold text-foreground">
          {resolved.decision === 'answer' ? `Answered: ${resolved.reason ?? ''}` : 'Dismissed'}
        </div>
      ) : (
        <div className="mt-2">
          <Button size="sm" variant="default" disabled={!complete || replyMutation.isPending} onClick={submit}>
            Answer
          </Button>
        </div>
      )}
    </div>
  )
}
