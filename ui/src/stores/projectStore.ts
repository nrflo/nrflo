import { create } from 'zustand'
import { setProject } from '@/api/client'
import { listProjects, type Project } from '@/api/projects'

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
        const firstProject = projects[0].id
        setProject(firstProject)
        set({ projects, currentProject: firstProject, projectsLoaded: true })
      } else {
        set({ projects, projectsLoaded: true })
      }
    } catch (e) {
      console.warn('Failed to load projects from API:', e)
      set({ projectsLoaded: true })
    }
  },
}))
