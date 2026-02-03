import { useNavigate } from 'react-router-dom'
import { ArrowLeft } from 'lucide-react'
import { Link } from 'react-router-dom'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { TicketForm, type TicketFormData } from '@/components/tickets/TicketForm'
import { useCreateTicket } from '@/hooks/useTickets'

export function CreateTicketPage() {
  const navigate = useNavigate()
  const createMutation = useCreateTicket()

  const handleSubmit = async (data: TicketFormData) => {
    await createMutation.mutateAsync({
      id: data.id,
      title: data.title,
      description: data.description,
      priority: data.priority,
      issue_type: data.issue_type,
      created_by: data.created_by,
    })
    navigate(`/tickets/${encodeURIComponent(data.id)}`)
  }

  return (
    <div className="max-w-2xl mx-auto space-y-6">
      <div className="flex items-center gap-4">
        <Link to="/tickets">
          <Button variant="ghost" size="icon">
            <ArrowLeft className="h-4 w-4" />
          </Button>
        </Link>
        <h1 className="text-2xl font-bold tracking-tight">Create Ticket</h1>
      </div>

      <Card>
        <CardHeader>
          <CardTitle>New Ticket</CardTitle>
        </CardHeader>
        <CardContent>
          <TicketForm
            onSubmit={handleSubmit}
            isSubmitting={createMutation.isPending}
          />
          {createMutation.isError && (
            <p className="mt-4 text-sm text-destructive">
              Error: {createMutation.error.message}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
