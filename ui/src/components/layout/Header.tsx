import { Link } from 'react-router-dom'
import { Search, Settings, LayoutDashboard, Ticket, FolderGit2, GitCommitHorizontal, BookOpen, Sun, Moon, Monitor } from 'lucide-react'
import { useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { Input } from '@/components/ui/Input'
import { ProjectSelect } from '@/components/ui/ProjectSelect'
import { DailyStats } from '@/components/layout/DailyStats'
import { RunningAgentsIndicator } from '@/components/layout/RunningAgentsIndicator'
import { useProjectStore } from '@/stores/projectStore'
import { useThemeStore } from '@/stores/themeStore'

export function Header() {
  const navigate = useNavigate()
  const [searchQuery, setSearchQuery] = useState('')
  const { currentProject, setCurrentProject, projects } = useProjectStore()
  const { theme, setTheme } = useThemeStore()

  const cycleTheme = () => {
    const next = theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system'
    setTheme(next)
  }
  const ThemeIcon = theme === 'light' ? Sun : theme === 'dark' ? Moon : Monitor

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
      <div className="flex h-14 items-center px-4 gap-2 md:gap-4">
        <Link to="/" className="flex items-center gap-2 font-semibold">
          <div className="flex h-8 w-8 items-center justify-center rounded-md bg-primary text-primary-foreground">
            {currentProjectObj?.name?.[0]?.toUpperCase() ?? 'N'}
          </div>
          <span className="hidden sm:inline-block">{currentProjectObj?.name ?? 'nrworkflow'}</span>
        </Link>

        <nav className="flex items-center gap-1 ml-4">
          <Link to="/" className="flex items-center p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Dashboard">
            <LayoutDashboard className="h-5 w-5" />
            <span className="hidden md:inline ml-1 text-xs">Dashboard</span>
          </Link>
          <Link to="/tickets" className="flex items-center p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Tickets">
            <Ticket className="h-5 w-5" />
            <span className="hidden md:inline ml-1 text-xs">Tickets</span>
          </Link>
          <Link to="/workflows" className="flex items-center p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Workflows">
            <FolderGit2 className="h-5 w-5" />
            <span className="hidden md:inline ml-1 text-xs">Workflows</span>
          </Link>
          {hasDefaultBranch && (
            <Link to="/git-status" className="flex items-center p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Git Status">
              <GitCommitHorizontal className="h-5 w-5" />
              <span className="hidden md:inline ml-1 text-xs">Git Status</span>
            </Link>
          )}
          <Link to="/documentation" className="flex items-center p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors" title="Documentation">
            <BookOpen className="h-5 w-5" />
            <span className="hidden md:inline ml-1 text-xs">Docs</span>
          </Link>
        </nav>

        <div className="flex-1" />

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

        <button
          onClick={cycleTheme}
          className="p-2 rounded-md text-muted-foreground hover:text-foreground hover:bg-muted transition-colors"
          title={`Theme: ${theme}`}
        >
          <ThemeIcon className="h-5 w-5" />
        </button>

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
