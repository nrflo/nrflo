import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Header } from './Header'

// Mock store - Header uses destructuring, not selector
let mockCurrentProject = 'test-project'
let mockProjects = [
  {
    id: 'test-project',
    name: 'Test Project',
    root_path: '/test',

    default_branch: 'main',
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  },
]

const mockSetCurrentProject = vi.fn()

vi.mock('@/stores/projectStore', () => ({
  useProjectStore: vi.fn(() => ({
    currentProject: mockCurrentProject,
    projects: mockProjects,
    setCurrentProject: mockSetCurrentProject,
  })),
}))

let mockTheme = 'system'
let mockSetTheme = vi.fn()

vi.mock('@/stores/themeStore', () => ({
  useThemeStore: vi.fn(() => ({
    theme: mockTheme,
    setTheme: mockSetTheme,
  })),
}))

// Mock DailyStats component
vi.mock('./DailyStats', () => ({
  DailyStats: () => <div data-testid="daily-stats">Daily Stats</div>,
}))

// Mock RunningAgentsIndicator component
vi.mock('./RunningAgentsIndicator', () => ({
  RunningAgentsIndicator: () => <div data-testid="running-agents">Running Agents</div>,
}))

// Mock ProjectSelect component
vi.mock('@/components/ui/ProjectSelect', () => ({
  ProjectSelect: ({ value, projects }: { value: string; projects: unknown[] }) => (
    <div data-testid="project-select">
      Project: {value} (Total: {projects.length})
    </div>
  ),
}))

function renderHeader(initialRoute = '/') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MemoryRouter initialEntries={[initialRoute]}>
        <Header />
      </MemoryRouter>
    </QueryClientProvider>
  )
}

describe('Header - Brand label', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockTheme = 'system'
    mockSetTheme = vi.fn()
    mockCurrentProject = 'test-project'
    mockProjects = [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
    
        default_branch: 'main',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]
  })

  it('shows project name and uppercased first-letter icon when project is selected', () => {
    renderHeader()

    const brandLink = screen.getByRole('link', { name: /test project/i })
    expect(brandLink).toBeInTheDocument()
    // icon letter is first char of project name, uppercased
    expect(brandLink).toHaveTextContent('T')
    // full project name in the brand label
    expect(brandLink).toHaveTextContent('Test Project')
  })

  it('falls back to N icon and nrworkflow text when no project is selected', () => {
    mockCurrentProject = ''

    renderHeader()

    const brandLink = screen.getByRole('link', { name: /nrworkflow/i })
    expect(brandLink).toBeInTheDocument()
    expect(brandLink).toHaveTextContent('N')
    expect(brandLink).toHaveTextContent('nrworkflow')
  })

  it('updates brand label when a different project is selected', () => {
    mockProjects = [
      {
        id: 'project-a',
        name: 'Alpha Suite',
        root_path: '/a',
    
        default_branch: null,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
      {
        id: 'project-b',
        name: 'Beta Tools',
        root_path: '/b',
    
        default_branch: null,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]
    mockCurrentProject = 'project-a'
    const { unmount } = renderHeader()

    expect(screen.getByRole('link', { name: /alpha suite/i })).toHaveTextContent('A')

    unmount()
    mockCurrentProject = 'project-b'
    renderHeader()

    expect(screen.getByRole('link', { name: /beta tools/i })).toHaveTextContent('B')
  })
})

describe('Header - Git Status Link', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    // Reset to project with default_branch
    mockCurrentProject = 'test-project'
    mockProjects = [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
    
        default_branch: 'main',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]
  })

  it('shows Git Status link when project has default_branch', () => {
    renderHeader()

    const gitStatusLink = screen.getByRole('link', { name: /git status/i })
    expect(gitStatusLink).toBeInTheDocument()
    expect(gitStatusLink).toHaveAttribute('href', '/git-status')
  })

  it('hides Git Status link when project has no default_branch', () => {
    mockProjects = [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
    
        default_branch: null,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]

    renderHeader()

    const gitStatusLink = screen.queryByRole('link', { name: /git status/i })
    expect(gitStatusLink).not.toBeInTheDocument()
  })

  it('hides Git Status link when no project is selected', () => {
    mockCurrentProject = ''

    renderHeader()

    const gitStatusLink = screen.queryByRole('link', { name: /git status/i })
    expect(gitStatusLink).not.toBeInTheDocument()
  })

  it('shows Git Status link after Workflows link', () => {
    renderHeader()

    const allLinks = screen.getAllByRole('link')
    const workflowsIndex = allLinks.findIndex((link) =>
      link.getAttribute('title') === 'Workflows'
    )
    const gitStatusIndex = allLinks.findIndex((link) =>
      link.getAttribute('title') === 'Git Status'
    )

    expect(workflowsIndex).toBeGreaterThan(-1)
    expect(gitStatusIndex).toBeGreaterThan(-1)
    expect(gitStatusIndex).toBeGreaterThan(workflowsIndex)
  })

  it('renders all main navigation links', () => {
    renderHeader()

    expect(screen.getByRole('link', { name: /dashboard/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /tickets/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /workflows/i })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /git status/i })).toBeInTheDocument()
  })
})

describe('Header - Icon-only nav links', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockCurrentProject = 'test-project'
    mockProjects = [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
    
        default_branch: 'main',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]
  })

  it('all 5 nav links render with correct titles, hrefs, and responsive text labels', () => {
    renderHeader()

    const expectedLinks = [
      { title: 'Dashboard', href: '/', label: 'Dashboard' },
      { title: 'Tickets', href: '/tickets', label: 'Tickets' },
      { title: 'Workflows', href: '/workflows', label: 'Workflows' },
      { title: 'Git Status', href: '/git-status', label: 'Git Status' },
      { title: 'Documentation', href: '/documentation', label: 'Docs' },
    ]

    for (const { title, href, label } of expectedLinks) {
      const link = screen.getByTitle(title).closest('a')!
      expect(link).toBeInTheDocument()
      expect(link).toHaveAttribute('href', href)
      // Icon + responsive text label (hidden md:inline)
      const span = link.querySelector('span.hidden')
      expect(span).toBeInTheDocument()
      expect(span?.textContent).toBe(label)
    }
  })
})

describe('Header - Theme toggle', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockTheme = 'system'
    mockSetTheme = vi.fn()
    mockCurrentProject = 'test-project'
    mockProjects = [
      {
        id: 'test-project',
        name: 'Test Project',
        root_path: '/test',
        default_branch: 'main',
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
      },
    ]
  })

  it.each([
    { theme: 'system', expectedTitle: 'Theme: system', nextTheme: 'light' },
    { theme: 'light', expectedTitle: 'Theme: light', nextTheme: 'dark' },
    { theme: 'dark', expectedTitle: 'Theme: dark', nextTheme: 'system' },
  ])('$theme mode: button shows "$expectedTitle" and click calls setTheme("$nextTheme")', async ({ theme, expectedTitle, nextTheme }) => {
    mockTheme = theme
    const user = userEvent.setup()
    renderHeader()

    const btn = screen.getByTitle(expectedTitle)
    expect(btn).toBeInTheDocument()
    await user.click(btn)
    expect(mockSetTheme).toHaveBeenCalledWith(nextTheme)
  })
})
