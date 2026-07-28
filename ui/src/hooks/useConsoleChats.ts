import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  getConsoleCatalog,
  getConsoleSkills,
  listConsoleChats,
  getConsoleChat,
  getConsoleChatMessages,
  createConsoleChat,
  sendConsoleChatMessage,
  replyConsoleChatApproval,
  closeConsoleChat,
  interruptConsoleChat,
  revokeConsoleChatSessionApproval,
  setConsoleChatYolo,
  switchConsoleChatModel,
  openConsoleChatHandsSibling,
  type SwitchConsoleChatModelRequest,
} from '@/api/consoleChats'
import { useProjectStore } from '@/stores/projectStore'
import type { ApprovalDecision, CreateConsoleChatRequest } from '@/types/consoleChat'

export const consoleChatKeys = {
  all: ['console-chats'] as const,
  list: () => [...consoleChatKeys.all, 'list'] as const,
  detail: (sid: string) => [...consoleChatKeys.all, 'detail', sid] as const,
  catalog: () => [...consoleChatKeys.all, 'catalog'] as const,
  skills: () => [...consoleChatKeys.all, 'skills'] as const,
  tools: (sid: string) => [...consoleChatKeys.all, 'tools', sid] as const,
}

// GET /console/catalog is project-scoped (X-Project) like the list below,
// so the key carries the project and the query waits for projectsLoaded.
// This is the same server-owned discovery surface the native TUI uses.
export function useConsoleCatalog() {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...consoleChatKeys.catalog(), project],
    queryFn: getConsoleCatalog,
    enabled: projectsLoaded,
  })
}

// GET /console/skills is project-scoped like the catalog above; fetched once
// per project (no polling) to back the composer's '/' suggestion dropdown.
export function useProjectSkills() {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...consoleChatKeys.skills(), project],
    queryFn: getConsoleSkills,
    enabled: projectsLoaded,
  })
}

// GET /console/chats is project-scoped via the X-Project header, so the key
// carries the project (switching projects must not serve the old one's list
// from cache) and the query waits for projectsLoaded — firing early would send
// an empty X-Project and 400.
export function useConsoleChats() {
  const project = useProjectStore((s) => s.currentProject)
  const projectsLoaded = useProjectStore((s) => s.projectsLoaded)
  return useQuery({
    queryKey: [...consoleChatKeys.list(), project],
    queryFn: listConsoleChats,
    enabled: projectsLoaded,
  })
}

export function useConsoleChat(sid: string | undefined) {
  return useQuery({
    queryKey: consoleChatKeys.detail(sid ?? ''),
    queryFn: () => getConsoleChat(sid!),
    enabled: !!sid,
  })
}

// History reuses the ['session-messages', sid] key so the WS messages.updated
// invalidation in useWebSocket/useWSReducer refreshes it for free.
export function useConsoleChatMessages(sid: string | undefined) {
  return useQuery({
    queryKey: ['session-messages', sid],
    queryFn: () => getConsoleChatMessages(sid!),
    enabled: !!sid,
  })
}

export function useCreateConsoleChat() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (req: CreateConsoleChatRequest) => createConsoleChat(req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.list() })
    },
  })
}

export function useSendConsoleChatMessage() {
  return useMutation({
    mutationFn: ({ sid, text }: { sid: string; text: string }) => sendConsoleChatMessage(sid, text),
  })
}

export function useReplyApproval() {
  return useMutation({
    mutationFn: ({ sid, aid, decision, answer }: { sid: string; aid: string; decision: ApprovalDecision; answer?: string }) =>
      replyConsoleChatApproval(sid, aid, decision, answer),
  })
}

export function useInterruptConsoleChat() {
  return useMutation({
    mutationFn: (sid: string) => interruptConsoleChat(sid),
  })
}

// Revoking also arrives as a console_chat.session_approvals push; the detail
// invalidation keeps a reload-seeded list in sync for tabs without the
// session channel open.
export function useRevokeSessionApproval() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ sid, tool }: { sid: string; tool: string }) => revokeConsoleChatSessionApproval(sid, tool),
    onSuccess: (_data, { sid }) => {
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.detail(sid) })
    },
  })
}

// Toggling also arrives as a console_chat.yolo push; the detail invalidation
// keeps a reload-seeded list in sync for tabs without the session channel open.
export function useSetYolo() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ sid, on }: { sid: string; on: boolean }) => setConsoleChatYolo(sid, on),
    onSuccess: (_data, { sid }) => {
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.detail(sid) })
    },
  })
}

export function useCloseConsoleChat() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (sid: string) => closeConsoleChat(sid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.list() })
    },
  })
}

// A model switch spawns a sibling chat rather than mutating the origin
// engine — invalidate the list/catalog so the new session shows up.
export function useSwitchConsoleChatModel() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: ({ sid, req }: { sid: string; req: SwitchConsoleChatModelRequest }) =>
      switchConsoleChatModel(sid, req),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.list() })
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.catalog() })
    },
  })
}

export function useOpenHandsSibling() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: (sid: string) => openConsoleChatHandsSibling(sid),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.list() })
      queryClient.invalidateQueries({ queryKey: consoleChatKeys.catalog() })
    },
  })
}
