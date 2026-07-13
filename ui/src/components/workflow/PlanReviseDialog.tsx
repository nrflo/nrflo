import { useState } from 'react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'
import { useRevisePlan } from '@/hooks/usePlan'
import { reportPlanError } from './PlanApprovalBanner'
import type { PlanQuestion, PlanTemplate } from '@/types/plan'

interface PlanReviseDialogProps {
  onClose: () => void
  instanceId: string
  revision: number
  questions?: PlanQuestion[]
  templates?: PlanTemplate[]
}

// Mounted only while open (see PlanApprovalBanner) so its useRevisePlan hook
// doesn't fire outside the revise flow.
export function PlanReviseDialog({ onClose, instanceId, revision, questions, templates }: PlanReviseDialogProps) {
  const [feedback, setFeedback] = useState('')
  const [answers, setAnswers] = useState<Record<string, string>>({})
  const reviseMutation = useRevisePlan()

  const handleSubmit = () => {
    const answerList = Object.entries(answers)
      .filter(([, answer]) => answer.trim().length > 0)
      .map(([question_id, answer]) => ({ question_id, answer }))

    reviseMutation.mutate(
      {
        instanceId,
        params: {
          revision,
          feedback: feedback.trim() || undefined,
          answers: answerList.length > 0 ? answerList : undefined,
        },
      },
      {
        onSuccess: () => onClose(),
        onError: (err) => reportPlanError('revise', err),
      }
    )
  }

  return (
    <Dialog open onClose={onClose} className="max-w-lg">
      <DialogHeader onClose={onClose}>
        <h3 className="text-lg font-semibold">Revise Plan (rev {revision})</h3>
      </DialogHeader>
      <DialogBody className="space-y-4">
        {templates && templates.length > 0 && (
          <div className="space-y-1.5">
            <p className="text-sm font-medium">Available templates</p>
            <ul className="space-y-1.5 rounded-md border border-border p-2 max-h-40 overflow-y-auto">
              {templates.map((t) => (
                <li key={t.id} className="text-xs">
                  <div className="flex items-center gap-1.5">
                    <Badge variant="secondary" className="text-xs">{t.id}</Badge>
                    <span className="text-muted-foreground">{t.model}</span>
                  </div>
                  {t.description && <p className="text-muted-foreground mt-0.5">{t.description}</p>}
                </li>
              ))}
            </ul>
          </div>
        )}
        <div>
          <label className="block text-sm font-medium mb-1.5">Feedback</label>
          <Textarea
            value={feedback}
            onChange={(e) => setFeedback(e.target.value)}
            placeholder="What should the planner change?"
            rows={4}
          />
        </div>
        {questions && questions.length > 0 && (
          <div className="space-y-3">
            <p className="text-sm font-medium">Open questions</p>
            {questions.map((q) => (
              <div key={q.id}>
                <label className="block text-xs text-muted-foreground mb-1">{q.question}</label>
                <Input
                  value={answers[q.id] ?? ''}
                  onChange={(e) => setAnswers((prev) => ({ ...prev, [q.id]: e.target.value }))}
                  placeholder="Your answer"
                />
              </div>
            ))}
          </div>
        )}
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" size="sm" onClick={onClose} disabled={reviseMutation.isPending}>
          Cancel
        </Button>
        <Button size="sm" onClick={handleSubmit} disabled={reviseMutation.isPending}>
          {reviseMutation.isPending ? <Spinner size="sm" className="mr-2" /> : null}
          Submit Revision
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
