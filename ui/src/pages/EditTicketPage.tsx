import { useMemo } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Spinner } from '@/components/ui/Spinner'
import { TicketForm, type TicketFormData } from '@/components/tickets/TicketForm'
import { useGoBack } from '@/hooks/useGoBack'
import { useTicket, useUpdateTicket, useTicketList } from '@/hooks/useTickets'

export function EditTicketPage() {
  const { id } = useParams<{ id: string }>()
  const decodedId = decodeURIComponent(id!)
  const navigate = useNavigate()
  const goBack = useGoBack(`/tickets/${encodeURIComponent(decodedId)}`)
  const { data: ticket, isLoading, error } = useTicket(decodedId)
  const updateMutation = useUpdateTicket()
  const { data: ticketData } = useTicketList()

  const parentOptions = useMemo(() => {
    if (!ticketData?.tickets) return []
    return ticketData.tickets
      .filter((t) => t.issue_type === 'epic' && t.id !== decodedId)
      .map((t) => ({ id: t.id, title: t.title }))
  }, [ticketData, decodedId])

  const handleSubmit = async (data: TicketFormData) => {
    try {
      await updateMutation.mutateAsync({
        id: decodedId,
        data: {
          title: data.title,
          description: data.description,
          priority: data.priority,
          issue_type: data.issue_type,
          parent_ticket_id: data.parent_ticket_id || undefined,
        },
      })
      navigate(`/tickets/${encodeURIComponent(decodedId)}`)
    } catch {
      // Error is displayed via updateMutation.isError state
    }
  }

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error || !ticket) {
    return (
      <div className="text-center py-12">
        <p className="text-destructive">
          {error ? `Error: ${error.message}` : 'Ticket not found'}
        </p>
        <Button variant="link" className="mt-4" onClick={goBack}>
          Back to tickets
        </Button>
      </div>
    )
  }

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-4">
        <Button variant="ghost" size="icon" onClick={goBack}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <h1 className="text-2xl font-bold tracking-tight">Edit Ticket</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{ticket.id}</CardTitle>
        </CardHeader>
        <CardContent>
          <TicketForm
            onSubmit={handleSubmit}
            isSubmitting={updateMutation.isPending}
            isEdit
            parentOptions={parentOptions}
            defaultValues={{
              id: ticket.id,
              title: ticket.title,
              description: ticket.description ?? undefined,
              priority: ticket.priority,
              issue_type: ticket.issue_type,
              created_by: ticket.created_by,
              parent_ticket_id: ticket.parent_ticket_id ?? '',
            }}
          />
          {updateMutation.isError && (
            <p className="mt-4 text-sm text-destructive">
              Error: {updateMutation.error.message}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
