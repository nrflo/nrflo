import { useForm, Controller, type SubmitHandler } from 'react-hook-form'
import { zodResolver } from '@hookform/resolvers/zod'
import { z } from 'zod'
import { Button } from '@/components/ui/Button'
import { Dropdown } from '@/components/ui/Dropdown'
import { Input } from '@/components/ui/Input'
import { Textarea } from '@/components/ui/Textarea'
import { Spinner } from '@/components/ui/Spinner'

const ticketSchema = z.object({
  id: z.string().optional().default(''),
  title: z.string().min(1, 'Title is required'),
  description: z.string().optional(),
  priority: z.coerce.number().min(1).max(4),
  issue_type: z.enum(['bug', 'feature', 'task', 'epic']),
  created_by: z.string().min(1, 'Created by is required'),
  parent_ticket_id: z.string().optional().default(''),
})

export type TicketFormData = z.infer<typeof ticketSchema>

export interface ParentOption {
  id: string
  title: string
}

interface TicketFormProps {
  onSubmit: (data: TicketFormData) => Promise<void>
  isSubmitting?: boolean
  defaultValues?: Partial<TicketFormData>
  isEdit?: boolean
  parentOptions?: ParentOption[]
}

export function TicketForm({
  onSubmit,
  isSubmitting,
  defaultValues,
  isEdit,
  parentOptions,
}: TicketFormProps) {
  const {
    register,
    handleSubmit,
    control,
    formState: { errors },
  } = useForm<TicketFormData>({
    resolver: zodResolver(ticketSchema) as never,
    defaultValues: {
      priority: 2,
      issue_type: 'task',
      created_by: 'ui',
      parent_ticket_id: '',
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
          <Controller
            name="issue_type"
            control={control}
            render={({ field }) => (
              <Dropdown
                value={field.value}
                onChange={field.onChange}
                options={[
                  { value: 'task', label: 'Task' },
                  { value: 'bug', label: 'Bug' },
                  { value: 'feature', label: 'Feature' },
                  { value: 'epic', label: 'Epic' },
                ]}
              />
            )}
          />
        </div>

        <div className="space-y-2">
          <label htmlFor="priority" className="text-sm font-medium">
            Priority
          </label>
          <Controller
            name="priority"
            control={control}
            render={({ field }) => (
              <Dropdown
                value={String(field.value)}
                onChange={field.onChange}
                options={[
                  { value: '1', label: '1 - Critical' },
                  { value: '2', label: '2 - High' },
                  { value: '3', label: '3 - Medium' },
                  { value: '4', label: '4 - Low' },
                ]}
              />
            )}
          />
        </div>
      </div>

      {parentOptions && parentOptions.length > 0 && (
        <div className="space-y-2">
          <label htmlFor="parent_ticket_id" className="text-sm font-medium">
            Parent Epic
          </label>
          <Controller
            name="parent_ticket_id"
            control={control}
            render={({ field }) => (
              <Dropdown
                value={field.value ?? ''}
                onChange={field.onChange}
                options={[
                  { value: '', label: 'None' },
                  ...parentOptions.map((opt) => ({
                    value: opt.id,
                    label: `${opt.id} - ${opt.title}`,
                  })),
                ]}
              />
            )}
          />
        </div>
      )}

      <div className="flex justify-end gap-3 pt-4">
        <Button type="submit" disabled={isSubmitting}>
          {isSubmitting && <Spinner size="sm" className="mr-2" />}
          {isEdit ? 'Update Ticket' : 'Create Ticket'}
        </Button>
      </div>
    </form>
  )
}
