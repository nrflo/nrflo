import { useForm, type SubmitHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Select } from '@/components/ui/Select'
import { Spinner } from '@/components/ui/Spinner'

const ticketSchema = z.object({
  id: z.string().min(1, 'ID is required'),
  title: z.string().min(1, 'Title is required'),
  description: z.string().optional(),
  priority: z.coerce.number().min(1).max(4),
  issue_type: z.enum(['bug', 'feature', 'task', 'epic']),
  created_by: z.string().min(1, 'Created by is required'),
})

export type TicketFormData = z.infer<typeof ticketSchema>

interface TicketFormProps {
  onSubmit: (data: TicketFormData) => Promise<void>
  isSubmitting?: boolean
  defaultValues?: Partial<TicketFormData>
  isEdit?: boolean
}

export function TicketForm({
  onSubmit,
  isSubmitting,
  defaultValues,
  isEdit,
}: TicketFormProps) {
  const {
    register,
    handleSubmit,
    formState: { errors },
  } = useForm<TicketFormData>({
    resolver: zodResolver(ticketSchema) as never,
    defaultValues: {
      priority: 2,
      issue_type: 'task',
      created_by: 'ui',
      ...defaultValues,
    },
  })

  const submitHandler: SubmitHandler<TicketFormData> = (data) => onSubmit(data)

  return (
    <form onSubmit={handleSubmit(submitHandler)} className="space-y-4">
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label htmlFor="id" className="text-sm font-medium">
            Ticket ID
          </label>
          <Input
            id="id"
            placeholder="e.g., PROJ-123 (auto-generated if empty)"
            disabled={isEdit}
            {...register('id')}
          />
          {errors.id && (
            <p className="text-xs text-destructive">{errors.id.message}</p>
          )}
        </div>

        <div className="space-y-2">
          <label htmlFor="created_by" className="text-sm font-medium">
            Created By
          </label>
          <Input
            id="created_by"
            placeholder="e.g., user@example.com"
            {...register('created_by')}
          />
          {errors.created_by && (
            <p className="text-xs text-destructive">{errors.created_by.message}</p>
          )}
        </div>
      </div>

      <div className="space-y-2">
        <label htmlFor="title" className="text-sm font-medium">
          Title
        </label>
        <Input
          id="title"
          placeholder="Brief description of the ticket"
          {...register('title')}
        />
        {errors.title && (
          <p className="text-xs text-destructive">{errors.title.message}</p>
        )}
      </div>

      <div className="space-y-2">
        <label htmlFor="description" className="text-sm font-medium">
          Description
        </label>
        <Textarea
          id="description"
          placeholder="Detailed description (optional)"
          rows={4}
          {...register('description')}
        />
      </div>

      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-2">
          <label htmlFor="issue_type" className="text-sm font-medium">
            Type
          </label>
          <Select id="issue_type" {...register('issue_type')}>
            <option value="task">Task</option>
            <option value="bug">Bug</option>
            <option value="feature">Feature</option>
            <option value="epic">Epic</option>
          </Select>
        </div>

        <div className="space-y-2">
          <label htmlFor="priority" className="text-sm font-medium">
            Priority
          </label>
          <Select id="priority" {...register('priority')}>
            <option value="1">1 - Critical</option>
            <option value="2">2 - High</option>
            <option value="3">3 - Medium</option>
            <option value="4">4 - Low</option>
          </Select>
        </div>
      </div>

      <div className="flex justify-end gap-3 pt-4">
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting && <Spinner size="sm" className="mr-2" />}
          {isEdit ? 'Update Ticket' : 'Create Ticket'}
        </Button>
      </div>
    </form>
  )
}
