import { useQuery } from '@tanstack/react-query'
import { listAuditLog } from '@/api/auditLog'
import type { AuditLogParams } from '@/api/auditLog'

export const auditKeys = {
  all: ['audit-log'] as const,
  list: (params: AuditLogParams) => [...auditKeys.all, 'list', params] as const,
}

export function useAuditLog(params: AuditLogParams = {}) {
  return useQuery({
    queryKey: auditKeys.list(params),
    queryFn: () => listAuditLog(params),
  })
}
