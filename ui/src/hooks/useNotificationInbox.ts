import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listInbox, markInboxRead, getInboxNotification } from '@/api/notificationInbox'
import { useProjectStore } from '@/stores/projectStore'
import { useConnectionsStore } from '@/stores/connectionsStore'

export function useNotificationInbox(global: boolean, offset = 0) {
  const project = useProjectStore((s) => s.currentProject)
  const connection = useConnectionsStore((s) => s.activeId)
  const queryClient = useQueryClient()
  const query = useQuery({
    queryKey: ['notification-inbox', connection, global ? 'global' : 'project', global ? '' : project, offset],
    queryFn: () => listInbox(global, offset),
    enabled: global || !!project,
  })
  const markRead = useMutation({
    mutationFn: (id?: string) => markInboxRead(global, id),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: ['notification-inbox'] }),
  })
  return { ...query, markRead }
}

export function useInboxNotification(global: boolean, id?: string) {
  const project = useProjectStore((s) => s.currentProject)
  const connection = useConnectionsStore((s) => s.activeId)
  return useQuery({
    queryKey: ['notification-inbox', connection, global ? 'global' : 'project', global ? '' : project, 'item', id],
    queryFn: () => getInboxNotification(global, id!),
    enabled: !!id && (global || !!project),
  })
}
