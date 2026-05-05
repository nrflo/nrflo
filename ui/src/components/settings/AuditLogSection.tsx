import { useState, useMemo } from 'react'
import { ChevronLeft, ChevronRight } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { Spinner } from '@/components/ui/Spinner'
import { useAuditLog } from '@/hooks/useAuditLog'
import { useUsers } from '@/hooks/useUsers'
import { formatDateTime } from '@/lib/utils'

// TODO(test-writer): smoke test: mock useAuditLog; assert rows render; pagination Next button advances page query param passed to hook

const PER_PAGE_OPTIONS = [
  { value: '50', label: '50 per page' },
  { value: '100', label: '100 per page' },
  { value: '200', label: '200 per page' },
]

export function AuditLogSection() {
  const [page, setPage] = useState(1)
  const [perPage, setPerPage] = useState(50)
  const [actionFilter, setActionFilter] = useState('')
  const [userIdFilter, setUserIdFilter] = useState('')

  const { data: usersData } = useUsers()
  const users = usersData?.users ?? []

  const userMap = useMemo(
    () => Object.fromEntries(users.map((u) => [u.id, u.display_name || u.email])),
    [users]
  )

  const userOptions = useMemo(
    () => [
      { value: '', label: 'All users' },
      ...users.map((u) => ({ value: u.id, label: u.display_name || u.email })),
    ],
    [users]
  )

  const { data, isLoading, error } = useAuditLog({
    page,
    per_page: perPage,
    user_id: userIdFilter || undefined,
    action: actionFilter.trim() || undefined,
  })

  const items = data?.items ?? []
  const total = data?.total ?? 0
  const totalPages = Math.max(1, Math.ceil(total / perPage))

  const handleActionChange = (v: string) => {
    setActionFilter(v)
    setPage(1)
  }

  const handleUserChange = (v: string) => {
    setUserIdFilter(v)
    setPage(1)
  }

  const handlePerPageChange = (v: string) => {
    setPerPage(Number(v))
    setPage(1)
  }

  return (
    <div className="space-y-4">
      <div>
        <h2 className="text-lg font-semibold">Audit Log</h2>
        <p className="text-sm text-muted-foreground">{total} total entries</p>
      </div>

      <div className="flex items-center gap-3 flex-wrap">
        <Input
          value={actionFilter}
          onChange={(e) => handleActionChange(e.target.value)}
          placeholder="Filter by action…"
          className="w-52"
        />
        <Dropdown
          value={userIdFilter}
          onChange={handleUserChange}
          options={userOptions}
          placeholder="All users"
          className="w-48"
        />
        <Dropdown
          value={String(perPage)}
          onChange={handlePerPageChange}
          options={PER_PAGE_OPTIONS}
          className="w-40"
        />
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load audit log'}
        </p>
      ) : items.length === 0 ? (
        <p className="text-center py-12 text-muted-foreground">No audit entries found.</p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-40">Timestamp</TableHead>
              <TableHead className="w-36">User</TableHead>
              <TableHead className="w-44">Action</TableHead>
              <TableHead>Resource</TableHead>
              <TableHead className="w-36">IP</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {items.map((entry) => (
              <TableRow key={entry.id}>
                <TableCell className="text-xs text-muted-foreground whitespace-nowrap">
                  {formatDateTime(entry.created_at)}
                </TableCell>
                <TableCell className="text-sm">
                  {entry.user_id ? (userMap[entry.user_id] ?? entry.user_id.slice(0, 8)) : '—'}
                </TableCell>
                <TableCell>
                  <code className="text-xs bg-muted px-1.5 py-0.5 rounded">{entry.action}</code>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {entry.resource_type
                    ? `${entry.resource_type}/${entry.resource_id || '—'}`
                    : '—'}
                </TableCell>
                <TableCell className="text-xs text-muted-foreground font-mono">
                  {entry.ip || '—'}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {!isLoading && !error && totalPages > 1 && (
        <div className="flex items-center justify-between">
          <p className="text-sm text-muted-foreground">
            Page {page} of {totalPages} · {total} entries
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page <= 1}
            >
              <ChevronLeft className="h-4 w-4" />
              Prev
            </Button>
            <Button
              variant="outline"
              size="sm"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page >= totalPages}
            >
              Next
              <ChevronRight className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )
}
