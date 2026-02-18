import { describe, it, expect, vi, beforeEach } from 'vitest'
import * as projectsApi from '@/api/projects'
import * as client from '@/api/client'

// Mock API modules
vi.mock('@/api/projects', () => ({
  listProjects: vi.fn(),
}))

vi.mock('@/api/client', () => ({
  setProject: vi.fn(),
}))

function makeProject(id: string, name = `Project ${id}`): projectsApi.Project {
  return {
    id,
    name,
    root_path: `/projects/${id}`,
    default_workflow: 'feature',
    default_branch: 'main',
    use_git_worktrees: false,
    use_docker_isolation: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
  }
}

function clearCookies() {
  document.cookie.split(';').forEach((c) => {
    const name = c.split('=')[0].trim()
    document.cookie = `${name}=; max-age=0; path=/`
  })
}

function getCookieValue(name: string): string | null {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : null
}

// Re-import the store fresh for each test to avoid state leaking
async function getStore() {
  // Use a fresh dynamic import to reset Zustand module state isn't possible in vitest without
  // resetting modules. Instead, directly test the store's actions by calling them.
  const { useProjectStore } = await import('@/stores/projectStore')
  return useProjectStore
}

describe('projectStore', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    clearCookies()
    // Reset Zustand store state between tests
    vi.resetModules()
  })

  describe('setCurrentProject', () => {
    it('writes project ID to nrwf_project cookie', async () => {
      const useProjectStore = await getStore()
      const { setCurrentProject } = useProjectStore.getState()

      setCurrentProject('my-project')

      expect(getCookieValue('nrwf_project')).toBe('my-project')
    })

    it('updates currentProject in store state', async () => {
      const useProjectStore = await getStore()
      const { setCurrentProject } = useProjectStore.getState()

      setCurrentProject('alpha')

      expect(useProjectStore.getState().currentProject).toBe('alpha')
    })

    it('calls setProject from api/client with the project ID', async () => {
      const useProjectStore = await getStore()
      const { setCurrentProject } = useProjectStore.getState()

      setCurrentProject('beta')

      expect(client.setProject).toHaveBeenCalledWith('beta')
    })

    it('overwrites cookie when called multiple times', async () => {
      const useProjectStore = await getStore()
      const { setCurrentProject } = useProjectStore.getState()

      setCurrentProject('first')
      setCurrentProject('second')

      expect(getCookieValue('nrwf_project')).toBe('second')
      expect(useProjectStore.getState().currentProject).toBe('second')
    })
  })

  describe('loadProjects — no cookie', () => {
    it('uses first project when no cookie is set', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a'), makeProject('proj-b')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(useProjectStore.getState().currentProject).toBe('proj-a')
    })

    it('writes the resolved project ID to cookie', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a'), makeProject('proj-b')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(getCookieValue('nrwf_project')).toBe('proj-a')
    })

    it('sets projectsLoaded to true', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(useProjectStore.getState().projectsLoaded).toBe(true)
    })
  })

  describe('loadProjects — valid cookie', () => {
    it('restores saved project when cookie matches a valid project ID', async () => {
      document.cookie = 'nrwf_project=proj-b; path=/'

      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a'), makeProject('proj-b'), makeProject('proj-c')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(useProjectStore.getState().currentProject).toBe('proj-b')
    })

    it('calls setProject with the cookie value when it is valid', async () => {
      document.cookie = 'nrwf_project=proj-b; path=/'

      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a'), makeProject('proj-b')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(client.setProject).toHaveBeenCalledWith('proj-b')
    })
  })

  describe('loadProjects — stale/invalid cookie', () => {
    it('falls back to first project when cookie references a deleted project', async () => {
      document.cookie = 'nrwf_project=deleted-project; path=/'

      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a'), makeProject('proj-b')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(useProjectStore.getState().currentProject).toBe('proj-a')
    })

    it('overwrites cookie with first project when stale cookie is found', async () => {
      document.cookie = 'nrwf_project=deleted-project; path=/'

      vi.mocked(projectsApi.listProjects).mockResolvedValue({
        projects: [makeProject('proj-a')],
      })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(getCookieValue('nrwf_project')).toBe('proj-a')
    })
  })

  describe('loadProjects — empty project list', () => {
    it('does not set currentProject when project list is empty', async () => {
      vi.mocked(projectsApi.listProjects).mockResolvedValue({ projects: [] })

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      // currentProject stays at initial value
      expect(useProjectStore.getState().currentProject).toBe('')
      expect(useProjectStore.getState().projectsLoaded).toBe(true)
    })
  })

  describe('loadProjects — API error', () => {
    it('sets projectsLoaded to true even on API failure', async () => {
      vi.mocked(projectsApi.listProjects).mockRejectedValue(new Error('Network error'))

      const useProjectStore = await getStore()
      await useProjectStore.getState().loadProjects()

      expect(useProjectStore.getState().projectsLoaded).toBe(true)
    })
  })
})
