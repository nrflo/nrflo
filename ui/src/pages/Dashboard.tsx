import { Link } from 'react-router-dom'
import {
  Ticket,
  Clock,
  CheckCircle,
  AlertCircle,
  ArrowRight,
  TrendingUp,
} from 'lucide-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/Card'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { TicketList } from '@/components/tickets/TicketList'
import { useStatus } from '@/hooks/useTickets'
import { useProjectStore } from '@/stores/projectStore'

interface StatCardProps {
  title: string
  value: number
  icon: React.ReactNode
  description?: string
  href?: string
}

function StatCard({ title, value, icon, description, href }: StatCardProps) {
  const content = (
    <Card className={`h-full ${href ? 'hover:border-primary/50 transition-colors' : ''}`}>
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {title}
        </CardTitle>
        {icon}
      </CardHeader>
      <CardContent>
        <div className="text-3xl font-bold">{value}</div>
        {description && (
          <p className="text-xs text-muted-foreground mt-1">{description}</p>
        )}
      </CardContent>
    </Card>
  )

  if (href) {
    return <Link to={href}>{content}</Link>
  }
  return content
}

export function Dashboard() {
  const currentProject = useProjectStore((s) => s.currentProject)
  const { data: status, isLoading, error } = useStatus()

  if (isLoading) {
    return (
      <div className="flex items-center justify-center py-12">
        <Spinner size="lg" />
      </div>
    )
  }

  if (error) {
    return (
      <div className="text-center py-12">
        <AlertCircle className="h-12 w-12 text-destructive mx-auto mb-4" />
        <h2 className="text-lg font-semibold">Failed to load dashboard</h2>
        <p className="text-muted-foreground mt-2">{error.message}</p>
        <p className="text-sm text-muted-foreground mt-4">
          Make sure the nrflo server is running: <code>nrflo_server serve</code>
        </p>
      </div>
    )
  }

  const counts = status?.counts || { open: 0, in_progress: 0, closed: 0, total: 0 }
  const readyCount = status?.ready_count || 0
  const pendingTickets = status?.pending_tickets || []
  const recentClosed = status?.recent_closed || []

  return (
    <div className="max-w-7xl mx-auto space-y-8">
      {/* Header */}
      <div>
        <h1 className="text-3xl font-bold tracking-tight">Dashboard</h1>
        <p className="text-muted-foreground">
          Project: <span className="font-medium">{currentProject}</span>
        </p>
      </div>

      {/* Stats Grid */}
      <div className="grid gap-4 md:grid-cols-2 lg:grid-cols-4">
        <StatCard
          title="Total Tickets"
          value={counts.total}
          icon={<Ticket className="h-4 w-4 text-muted-foreground" />}
          href="/tickets"
        />
        <StatCard
          title="Open"
          value={counts.open}
          icon={<AlertCircle className="h-4 w-4 text-blue-500" />}
          description={`${readyCount} ready to work on`}
          href="/tickets?status=open"
        />
        <StatCard
          title="In Progress"
          value={counts.in_progress}
          icon={<Clock className="h-4 w-4 text-yellow-500" />}
          href="/tickets?status=in_progress"
        />
        <StatCard
          title="Closed"
          value={counts.closed}
          icon={<CheckCircle className="h-4 w-4 text-green-500" />}
          href="/tickets?status=closed"
        />
      </div>

      {/* Main Content Grid */}
      <div className="grid gap-6 lg:grid-cols-2">
        {/* Pending Tickets */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <TrendingUp className="h-5 w-5" />
              Active Tickets
            </CardTitle>
            <Link to="/tickets?status=open">
              <Button variant="ghost" size="sm">
                View all
                <ArrowRight className="h-4 w-4 ml-1" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            <TicketList
              tickets={pendingTickets.slice(0, 5)}
              isLoading={false}
              error={null}
              emptyMessage="No active tickets"
            />
          </CardContent>
        </Card>

        {/* Recently Closed */}
        <Card>
          <CardHeader className="flex flex-row items-center justify-between">
            <CardTitle className="flex items-center gap-2">
              <CheckCircle className="h-5 w-5 text-green-500" />
              Recently Closed
            </CardTitle>
            <Link to="/tickets?status=closed">
              <Button variant="ghost" size="sm">
                View all
                <ArrowRight className="h-4 w-4 ml-1" />
              </Button>
            </Link>
          </CardHeader>
          <CardContent>
            <TicketList
              tickets={recentClosed.slice(0, 5)}
              isLoading={false}
              error={null}
              emptyMessage="No recently closed tickets"
            />
          </CardContent>
        </Card>
      </div>

      {/* Quick Actions */}
      <Card>
        <CardHeader>
          <CardTitle>Quick Actions</CardTitle>
        </CardHeader>
        <CardContent className="flex gap-4">
          <Link to="/tickets/new">
            <Button>Create New Ticket</Button>
          </Link>
          <Link to="/tickets">
            <Button variant="outline">View All Tickets</Button>
          </Link>
        </CardContent>
      </Card>
    </div>
  )
}
