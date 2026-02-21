import { Link } from 'react-router-dom'
import { Search, Settings, LayoutDashboard, Ticket, FolderGit2, GitCommitHorizontal, BookOpen, Terminal } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/Input'
import { ProjectSelect } from '@/components/ui/ProjectSelect'
import { DailyStats } from '@/components/layout/DailyStats'
import { UsageLimits } from '@/components/layout/UsageLimits'
import { RunningAgentsIndicator } from '@/components/layout/RunningAgentsIndicator'
import { useProjectStore } from '@/stores/projectStore'

export function Header() {
  const navigate = useNavigate()
  const [searchQuery, setSearchQuery] = useState('')
  const { currentProject, setCurrentProject, projects } = useProjectStore()

  const currentProjectObj = useMemo(
    () => projects.find((p) => p.id === currentProject),
    [projects, currentProject],
  )
  const hasDefaultBranch = !!currentProjectObj?.default_branch

  const handleSearch = (e: React.FormEvent) => {
    e.preventDefault()
    if (searchQuery.trim()) {
      navigate(`/tickets?search=${encodeURIComponent(searchQuery.trim())}`)
      setSearchQuery('')
    }
  }

  return (
    <header className="sticky top-0 z-50 w-full border-b border-border bg-background/95 backdrop-blur supports-[backdrop-filter]:bg-background/60">
      <div className="flex h-14 items-center px-4 gap-4">
        <Link to="/" className="flex items-center gap-2 font-semibold">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            {currentProjectObj?.name?.[0]?.toUpperCase() ?? 'N'}
          </div>
          <span className="hidden sm:inline-block">{currentProjectObj?.name ?? 'nrworkflow'}</span>
        </Link>

        <nav className="flex items-center gap-1 ml-4">
          <Link to="/" className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Dashboard">
            <LayoutDashboard className="h-5 w-5" />
          </Link>
          <Link to="/tickets" className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Tickets">
            <Ticket className="h-5 w-5" />
          </Link>
          <Link to="/workflows" className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Workflows">
            <FolderGit2 className="h-5 w-5" />
          </Link>
          {hasDefaultBranch && (
            <Link to="/git-status" className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Git Status">
              <GitCommitHorizontal className="h-5 w-5" />
            </Link>
          )}
          <Link to="/documentation" className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Documentation">
            <BookOpen className="h-5 w-5" />
          </Link>
          <Link to="/logs" className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Logs">
            <Terminal className="h-5 w-5" />
          </Link>
        </nav>

        <div className="flex-1" />

        <UsageLimits />
        <DailyStats />

        <div className="flex-1" />

        <RunningAgentsIndicator />

        <form onSubmit={handleSearch} className="hidden sm:block">
          <div className="relative">
            <Search className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              type="search"
              placeholder="Search tickets..."
              className="pl-8 w-64"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
            />
          </div>
        </form>

        <ProjectSelect
          value={currentProject}
          onChange={(value) => {
            setCurrentProject(value)
            navigate('/')
          }}
          projects={projects}
        />

        <Link
          to="/settings"
          className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
          title="Settings"
        >
          <Settings className="h-5 w-5" />
        </Link>
      </div>
    </header>
  )
}
