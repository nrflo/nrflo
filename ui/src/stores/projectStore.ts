import { create } from 'zustand'
import { setProject } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'

function getCookie(name: string): string | null {
  const match = document.cookie.match(new RegExp('(?:^|; )' + name + '=([^;]*)'))
  return match ? decodeURIComponent(match[1]) : null
}

function setCookie(name: string, value: string): void {
  document.cookie = `${name}=${encodeURIComponent(value)}; path=/; SameSite=Lax; max-age=31536000`
}

interface ProjectState {
  currentProject: string
  projects: Project[]
  projectsLoaded: boolean
  setCurrentProject: (projectId: string) => void
  setProjects: (projects: Project[]) => void
  loadProjects: () => Promise<void>
}

export const useProjectStore = create<ProjectState>()((set) => ({
  currentProject: '',
  projects: [],
  projectsLoaded: false,
  setCurrentProject: (projectId: string) => {
    setProject(projectId)
    setCookie('nrf_project', projectId)
    set({ currentProject: projectId })
  },
  setProjects: (projects: Project[]) => {
    set({ projects })
  },
  loadProjects: async () => {
    try {
      const response = await listProjects()
      const projects = response.projects || []
      if (projects.length > 0) {
        const saved = getCookie('nrf_project')
        const resolved = saved && projects.some((p) => p.id === saved) ? saved : projects[0].id
        setProject(resolved)
        setCookie('nrf_project', resolved)
        set({ projects, currentProject: resolved, projectsLoaded: true })
      } else {
        set({ projects, projectsLoaded: true })
      }
    } catch (e) {
      console.warn('Failed to load projects from API:', e)
      set({ projectsLoaded: true })
    }
  },
}))
