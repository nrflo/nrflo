import { useState } from 'react'
import { Dialog, DialogHeader, DialogBody, DialogFooter } from '@/components/ui/Dialog'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { ApiError } from '@/api/client'
import { useCreateUser, useUpdateUser, useResetUserPassword } from '@/hooks/useUsers'
import type { User } from '@/types/user'

const ROLE_OPTIONS = [
  { value: 'admin', label: 'Admin' },
  { value: 'viewer', label: 'Viewer' },
]

const STATUS_OPTIONS = [
  { value: 'active', label: 'Active' },
  { value: 'disabled', label: 'Disabled' },
]

function mapApiError(e: unknown): string {
  if (e instanceof ApiError) {
    if (e.message === 'email_exists') return 'A user with this email already exists.'
    if (e.message === 'last_admin') return 'Cannot demote or disable the last admin user.'
    if (e.message === 'cannot_delete_self') return 'You cannot delete your own account.'
    return e.message
  }
  return 'An unexpected error occurred.'
}

interface CreateUserDialogProps {
  open: boolean
  onClose: () => void
}

export function CreateUserDialog({ open, onClose }: CreateUserDialogProps) {
  const [email, setEmail] = useState('')
  const [displayName, setDisplayName] = useState('')
  const [password, setPassword] = useState('')
  const [role, setRole] = useState('viewer')
  const [error, setError] = useState<string | null>(null)
  const createMutation = useCreateUser()

  const handleClose = () => {
    setEmail('')
    setDisplayName('')
    setPassword('')
    setRole('viewer')
    setError(null)
    onClose()
  }

  const handleSubmit = async () => {
    setError(null)
    try {
      await createMutation.mutateAsync({
        email,
        display_name: displayName,
        password,
        role: role as 'admin' | 'viewer',
      })
      handleClose()
    } catch (e) {
      setError(mapApiError(e))
    }
  }

  return (
    <Dialog open={open} onClose={handleClose} className="max-w-md">
      <DialogHeader onClose={handleClose}>Create User</DialogHeader>
      <DialogBody className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="space-y-1">
          <label className="text-sm font-medium">Email</label>
          <Input value={email} onChange={(e) => setEmail(e.target.value)} placeholder="user@example.com" type="email" />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Display Name</label>
          <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Jane Doe" />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Password</label>
          <Input value={password} onChange={(e) => setPassword(e.target.value)} type="password" placeholder="8-128 characters" />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Role</label>
          <Dropdown value={role} onChange={setRole} options={ROLE_OPTIONS} />
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose} disabled={createMutation.isPending}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          disabled={createMutation.isPending || !email || !displayName || !password}
        >
          {createMutation.isPending ? 'Creating…' : 'Create User'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}

interface EditUserDialogProps {
  user: User
  onClose: () => void
}

export function EditUserDialog({ user, onClose }: EditUserDialogProps) {
  const [displayName, setDisplayName] = useState(user.display_name)
  const [role, setRole] = useState(user.role)
  const [status, setStatus] = useState(user.status)
  const [error, setError] = useState<string | null>(null)
  const updateMutation = useUpdateUser()

  const handleSubmit = async () => {
    setError(null)
    try {
      await updateMutation.mutateAsync({
        id: user.id,
        data: {
          display_name: displayName,
          role: role as 'admin' | 'viewer',
          status: status as 'active' | 'disabled',
        },
      })
      onClose()
    } catch (e) {
      setError(mapApiError(e))
    }
  }

  return (
    <Dialog open onClose={onClose} className="max-w-md">
      <DialogHeader onClose={onClose}>Edit User — {user.email}</DialogHeader>
      <DialogBody className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="space-y-1">
          <label className="text-sm font-medium">Display Name</label>
          <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} placeholder="Jane Doe" />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Role</label>
          <Dropdown value={role} onChange={(v) => setRole(v as 'admin' | 'viewer')} options={ROLE_OPTIONS} />
        </div>
        <div className="space-y-1">
          <label className="text-sm font-medium">Status</label>
          <Dropdown value={status} onChange={(v) => setStatus(v as 'active' | 'disabled')} options={STATUS_OPTIONS} />
        </div>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={onClose} disabled={updateMutation.isPending}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={updateMutation.isPending || !displayName}>
          {updateMutation.isPending ? 'Saving…' : 'Save Changes'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}

interface ResetPasswordDialogProps {
  user: User
  onClose: () => void
}

export function ResetPasswordDialog({ user, onClose }: ResetPasswordDialogProps) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState<string | null>(null)
  const resetMutation = useResetUserPassword()

  const handleClose = () => {
    setPassword('')
    setError(null)
    onClose()
  }

  const handleSubmit = async () => {
    setError(null)
    try {
      await resetMutation.mutateAsync({ id: user.id, data: { new_password: password } })
      handleClose()
    } catch (e) {
      setError(mapApiError(e))
    }
  }

  return (
    <Dialog open onClose={handleClose} className="max-w-md">
      <DialogHeader onClose={handleClose}>Reset Password — {user.display_name}</DialogHeader>
      <DialogBody className="space-y-4">
        {error && <p className="text-sm text-destructive">{error}</p>}
        <div className="space-y-1">
          <label className="text-sm font-medium">New Password</label>
          <Input
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            type="password"
            placeholder="8-128 characters"
          />
        </div>
        <p className="text-xs text-muted-foreground">
          User will be required to change their password on next login.
        </p>
      </DialogBody>
      <DialogFooter>
        <Button variant="outline" onClick={handleClose} disabled={resetMutation.isPending}>Cancel</Button>
        <Button onClick={handleSubmit} disabled={resetMutation.isPending || password.length < 8}>
          {resetMutation.isPending ? 'Resetting…' : 'Reset Password'}
        </Button>
      </DialogFooter>
    </Dialog>
  )
}
