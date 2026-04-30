import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  listScheduledTasks,
  createScheduledTask,
  updateScheduledTask,
  deleteScheduledTask,
  listScheduleRuns,
  runScheduledTaskNow,
} from '@/api/scheduledTasks'
import type {
  ScheduledTaskCreateRequest,
  ScheduledTaskUpdateRequest,
} from '@/types/schedules'
import { useProjectStore } from '@/stores/projectStore'

export const scheduleKeys = {
  all: ['scheduled-tasks'] as const,
  lists: () => [...scheduleKeys.all, 'list'] as const,
  runs: (taskId: string) => ['schedule-runs', taskId] as const,
  runsPage: (taskId: string, page: number) => [...scheduleKeys.runs(taskId), page] as const,
}

const RUNS_PAGE_SIZE = 20

export function useScheduledTasks() {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...scheduleKeys.lists(), project],
    queryFn: listScheduledTasks,
    enabled: projectsLoaded,
  })
}

export function useScheduleRuns(taskId: string, page: number) {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  const offset = page * RUNS_PAGE_SIZE
  return useQuery({
    queryKey: [...scheduleKeys.runsPage(taskId, page), project],
    queryFn: () => listScheduleRuns(taskId, { limit: RUNS_PAGE_SIZE, offset }),
    enabled: projectsLoaded && !!taskId,
  })
}

export function useCreateScheduledTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (data: ScheduledTaskCreateRequest) => createScheduledTask(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: scheduleKeys.all })
    },
  })
}

export function useUpdateScheduledTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: ScheduledTaskUpdateRequest }) =>
      updateScheduledTask(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: scheduleKeys.all })
    },
  })
}

export function useDeleteScheduledTask() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => deleteScheduledTask(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: scheduleKeys.all })
    },
  })
}

export function useRunScheduleNow() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (id: string) => runScheduledTaskNow(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: scheduleKeys.all })
    },
  })
}
