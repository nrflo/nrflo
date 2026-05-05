import { useState } from 'react'
import { Pencil, Trash2, Plus, KeyRound, Users } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { ConfirmDialog } from '@/components/ui/ConfirmDialog'
import { Spinner } from '@/components/ui/Spinner'
import { ApiError } from '@/api/client'
import { useUsers, useDeleteUser } from '@/hooks/useUsers'
import { formatRelativeTime } from '@/lib/utils'
import { CreateUserDialog, EditUserDialog, ResetPasswordDialog } from './UsersPageDialogs'
import type { User } from '@/types/user'

// TODO(test-writer): smoke test: mock useUsers + mutations; assert table renders rows; clicking Create opens modal; submit triggers createUser mutation

function mapDeleteError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.message === 'last_admin') return 'Cannot delete the last admin user.'
    if (e.message === 'cannot_delete_self') return 'You cannot delete your own account.'
    if (e.message === 'system_user') return 'This is a system user and cannot be deleted.'
    return e.message
  }
  return 'Failed to delete user.'
}

export function UsersPage() {
  const [showCreate, setShowCreate] = useState(false)
  const [editTarget, setEditTarget] = useState<User | null>(null)
  const [resetTarget, setResetTarget] = useState<User | null>(null)
  const [deleteTargetId, setDeleteTargetId] = useState<string | null>(null)
  const [deleteError, setDeleteError] = useState<string | null>(null)

  const { data, isLoading, error } = useUsers()
  const users = data?.users ?? []
  const deleteMutation = useDeleteUser()

  const handleDelete = async () => {
    if (!deleteTargetId) return
    setDeleteError(null)
    try {
      await deleteMutation.mutateAsync(deleteTargetId)
      setDeleteTargetId(null)
    } catch (e) {
      setDeleteError(mapDeleteError(e))
    }
  }

  return (
    <div className="p-6 max-w-5xl mx-auto space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="flex items-center gap-2 text-2xl font-bold tracking-tight">
            <Users className="h-6 w-6" />
            Users
          </h1>
          <p className="text-muted-foreground">{users.length} user{users.length !== 1 ? 's' : ''}</p>
        </div>
        <Button onClick={() => setShowCreate(true)}>
          <Plus className="h-4 w-4 mr-2" />
          Create User
        </Button>
      </div>

      {isLoading ? (
        <div className="flex justify-center py-12">
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <p className="text-destructive text-sm">
          {error instanceof Error ? error.message : 'Failed to load users'}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Email</TableHead>
              <TableHead>Display Name</TableHead>
              <TableHead className="w-24">Role</TableHead>
              <TableHead className="w-24">Status</TableHead>
              <TableHead className="w-36">Last Login</TableHead>
              <TableHead className="w-36">Must Change Pwd</TableHead>
              <TableHead className="w-28" />
            </TableRow>
          </TableHeader>
          <TableBody>
            {users.map((user) => (
              <TableRow key={user.id}>
                <TableCell className="font-medium">{user.email}</TableCell>
                <TableCell>{user.display_name}</TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <span className={`text-xs px-1.5 py-0.5 rounded ${
                      user.role === 'admin'
                        ? 'bg-primary/10 text-primary'
                        : 'bg-muted text-muted-foreground'
                    }`}>
                      {user.role}
                    </span>
                    {user.system && (
                      <span className="text-xs px-1.5 py-0.5 rounded bg-muted text-muted-foreground">
                        System
                      </span>
                    )}
                  </div>
                </TableCell>
                <TableCell>
                  <span className={`text-xs px-1.5 py-0.5 rounded ${
                    user.status === 'active'
                      ? 'bg-green-100 text-green-700 dark:bg-green-900 dark:text-green-300'
                      : 'bg-red-100 text-red-700 dark:bg-red-900 dark:text-red-300'
                  }`}>
                    {user.status}
                  </span>
                </TableCell>
                <TableCell className="text-sm text-muted-foreground">
                  {user.last_login_at ? formatRelativeTime(user.last_login_at) : '—'}
                </TableCell>
                <TableCell className="text-sm">
                  {user.must_change_password ? (
                    <span className="text-xs px-1.5 py-0.5 rounded bg-amber-100 text-amber-700 dark:bg-amber-900 dark:text-amber-300">
                      yes
                    </span>
                  ) : '—'}
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => setEditTarget(user)}
                      className="p-1 text-muted-foreground hover:text-foreground transition-colors"
                      title="Edit"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => setResetTarget(user)}
                      className="p-1 text-muted-foreground hover:text-foreground transition-colors"
                      title="Reset password"
                    >
                      <KeyRound className="h-3.5 w-3.5" />
                    </button>
                    {!user.system && (
                      <button
                        onClick={() => { setDeleteTargetId(user.id); setDeleteError(null) }}
                        className="p-1 text-muted-foreground hover:text-destructive transition-colors"
                        title="Delete"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      {showCreate && (
        <CreateUserDialog open onClose={() => setShowCreate(false)} />
      )}
      {editTarget && (
        <EditUserDialog key={editTarget.id} user={editTarget} onClose={() => setEditTarget(null)} />
      )}
      {resetTarget && (
        <ResetPasswordDialog key={resetTarget.id} user={resetTarget} onClose={() => setResetTarget(null)} />
      )}
      <ConfirmDialog
        open={!!deleteTargetId}
        onClose={() => { setDeleteTargetId(null); setDeleteError(null) }}
        onConfirm={handleDelete}
        title="Delete User"
        message={deleteError ?? `Delete this user permanently? This action cannot be undone.`}
        confirmLabel="Delete"
        variant="destructive"
      />
    </div>
  )
}
