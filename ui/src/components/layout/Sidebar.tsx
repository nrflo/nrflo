import { Link, useLocation } from 'react-router-dom'
import {
  LayoutDashboard,
  Ticket,
  Plus,
  CheckCircle,
  Clock,
  AlertCircle,
  Lock,
  FolderGit2,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { useStatus } from '@/hooks/useTickets'
import { Spinner } from '@/components/ui/Spinner'

interface NavItemProps {
  to: string
  icon: React.ReactNode
  label: string
  count?: number
  active?: boolean
  indicator?: React.ReactNode
}

function NavItem({ to, icon, label, count, active, indicator }: NavItemProps) {
  return (
    <Link
      to={to}
      className={cn(
        'flex items-center gap-3 rounded-lg px-3 py-2 text-sm transition-colors',
        active
          ? 'bg-muted text-foreground'
          : 'text-muted-foreground hover:bg-muted hover:text-foreground'
      )}
    >
      {icon}
      <span className="flex-1">{label}</span>
      {indicator}
      {count !== undefined && (
        <span className="text-xs text-muted-foreground">{count}</span>
      )}
    </Link>
  )
}

export function Sidebar() {
  const location = useLocation()
  const { data: status } = useStatus()

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  return (
    <aside className="hidden lg:block w-64 border-r border-border bg-background">
      <nav className="flex flex-col gap-2 p-4">
        <NavItem
          to="/"
          icon={<LayoutDashboard className="h-4 w-4" />}
          label="Dashboard"
          active={isActive('/')}
        />
        <NavItem
          to="/tickets"
          icon={<Ticket className="h-4 w-4" />}
          label="All Tickets"
          count={status?.counts.total}
          active={location.pathname === '/tickets' && !location.search}
        />
        <NavItem
          to="/tickets/new"
          icon={<Plus className="h-4 w-4" />}
          label="New Ticket"
          active={isActive('/tickets/new')}
        />
        <NavItem
          to="/project-workflows"
          icon={<FolderGit2 className="h-4 w-4" />}
          label="Project Workflows"
          active={isActive('/project-workflows')}
        />
        <div className="mt-4 mb-2 px-3 text-xs font-semibold text-muted-foreground uppercase tracking-wider">
          By Status
        </div>
        <NavItem
          to="/tickets?status=open"
          icon={<AlertCircle className="h-4 w-4 text-blue-500" />}
          label="Open"
          count={status?.counts.open}
          active={location.search.includes('status=open')}
        />
        <NavItem
          to="/tickets?status=in_progress"
          icon={<Clock className="h-4 w-4 text-yellow-500" />}
          label="In Progress"
          count={status?.counts.in_progress}
          active={location.search.includes('status=in_progress')}
          indicator={status?.counts.in_progress ? <Spinner size="sm" /> : undefined}
        />
        <NavItem
          to="/tickets?status=closed"
          icon={<CheckCircle className="h-4 w-4 text-green-500" />}
          label="Closed"
          count={status?.counts.closed}
          active={location.search.includes('status=closed')}
        />
        <NavItem
          to="/tickets?status=blocked"
          icon={<Lock className="h-4 w-4 text-orange-500" />}
          label="Blocked"
          count={status?.counts.blocked}
          active={location.search.includes('status=blocked')}
        />
      </nav>
    </aside>
  )
}
