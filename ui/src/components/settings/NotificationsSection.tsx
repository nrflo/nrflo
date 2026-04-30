import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, Send, MessageSquare } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { Toggle } from '@/components/ui/Toggle'
import {
  listNotificationChannels,
  createNotificationChannel,
  updateNotificationChannel,
  deleteNotificationChannel,
  testNotificationChannel,
} from '@/api/notifications'
import type { NotificationChannel } from '@/types/notifications'
import {
  NotificationChannelForm,
  emptyChannelForm,
  channelToFormData,
  buildConfig,
  type ChannelFormData,
} from './NotificationChannelForm'

const notificationKeys = {
  all: ['notification-channels'] as const,
  list: () => [...notificationKeys.all, 'list'] as const,
  detail: (id: number) => [...notificationKeys.all, id] as const,
  deliveries: (channelId: number) => ['notification-deliveries', channelId] as const,
}

const KIND_LABELS: Record<string, string> = { slack: 'Slack', telegram: 'Telegram' }

function KindBadge({ kind }: { kind: string }) {
  return (
    <Badge
      variant="secondary"
      className={`text-xs ${kind === 'slack' ? 'bg-yellow-500/20 text-yellow-300 border-yellow-500/30' : 'bg-blue-500/20 text-blue-300 border-blue-500/30'}`}
    >
      <MessageSquare className="h-3 w-3 mr-1" />
      {KIND_LABELS[kind] ?? kind}
    </Badge>
  )
}

export function NotificationsSection() {
  const queryClient = useQueryClient()

  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<number | null>(null)
  const [formData, setFormData] = useState<ChannelFormData>(emptyChannelForm())
  const [deleteConfirm, setDeleteConfirm] = useState<number | null>(null)

  const { data: channels = [], isLoading, error } = useQuery({
    queryKey: notificationKeys.list(),
    queryFn: listNotificationChannels,
  })

  const createMutation = useMutation({
    mutationFn: (data: ChannelFormData) =>
      createNotificationChannel({
        name: data.name.trim(),
        kind: data.kind,
        enabled: data.enabled,
        config: buildConfig(data),
        event_types: data.eventTypes,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all })
      setIsCreating(false)
      setFormData(emptyChannelForm())
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: number; data: ChannelFormData }) =>
      updateNotificationChannel(id, {
        name: data.name.trim(),
        enabled: data.enabled,
        config: buildConfig(data),
        event_types: data.eventTypes,
      }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all })
      setEditingId(null)
      setFormData(emptyChannelForm())
    },
  })

  const toggleMutation = useMutation({
    mutationFn: ({ id, enabled }: { id: number; enabled: boolean }) =>
      updateNotificationChannel(id, { enabled }),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all })
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: number) => deleteNotificationChannel(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: notificationKeys.all })
      setDeleteConfirm(null)
    },
  })

  const testMutation = useMutation({
    mutationFn: (id: number) => testNotificationChannel(id),
    onSuccess: () => toast.success('Test notification sent'),
    onError: (err: Error) => toast.error(`Test failed: ${err.message}`),
  })

  const handleStartCreate = () => {
    setIsCreating(true)
    setEditingId(null)
    setFormData(emptyChannelForm())
  }

  const handleStartEdit = (ch: NotificationChannel) => {
    setEditingId(ch.id)
    setIsCreating(false)
    setFormData(channelToFormData(ch))
  }

  const handleCancel = () => {
    setIsCreating(false)
    setEditingId(null)
    setFormData(emptyChannelForm())
  }

  const handleSaveCreate = () => {
    if (!formData.name.trim() || formData.eventTypes.length === 0) return
    createMutation.mutate(formData)
  }

  const handleSaveEdit = () => {
    if (!editingId) return
    updateMutation.mutate({ id: editingId, data: formData })
  }

  const renderChannelRow = (ch: NotificationChannel) => (
    <div key={ch.id} className="border rounded-lg p-4">
      {editingId === ch.id ? (
        <NotificationChannelForm
          formData={formData}
          setFormData={setFormData}
          onCancel={handleCancel}
          onSave={handleSaveEdit}
          mutation={updateMutation}
          editingChannel={ch}
        />
      ) : deleteConfirm === ch.id ? (
        <div className="flex items-center justify-between">
          <div className="text-sm">
            Are you sure you want to delete{' '}
            <span className="font-semibold">{ch.name}</span>?
          </div>
          <div className="flex gap-2">
            <Button variant="ghost" onClick={() => setDeleteConfirm(null)}>
              Cancel
            </Button>
            <Button
              variant="destructive"
              onClick={() => deleteMutation.mutate(ch.id)}
              disabled={deleteMutation.isPending}
            >
              {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
            </Button>
          </div>
        </div>
      ) : (
        <div className="flex items-center justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <Toggle
              checked={ch.enabled}
              onChange={(val) => toggleMutation.mutate({ id: ch.id, enabled: val })}
              disabled={toggleMutation.isPending}
            />
            <div className="min-w-0">
              <div className="flex items-center gap-2 flex-wrap">
                <span className="font-medium">{ch.name}</span>
                <KindBadge kind={ch.kind} />
              </div>
              {ch.event_types?.length > 0 && (
                <div className="flex gap-1 flex-wrap mt-1">
                  {ch.event_types.map((et) => (
                    <span key={et} className="text-xs text-muted-foreground font-mono bg-muted px-1.5 py-0.5 rounded">
                      {et}
                    </span>
                  ))}
                </div>
              )}
            </div>
          </div>
          <div className="flex gap-1 shrink-0">
            <Button variant="ghost" size="icon" onClick={() => handleStartEdit(ch)}>
              <Pencil className="h-4 w-4" />
            </Button>
            <Button
              variant="ghost"
              size="icon"
              onClick={() => testMutation.mutate(ch.id)}
              disabled={testMutation.isPending}
              title="Send test notification"
            >
              <Send className="h-4 w-4" />
            </Button>
            <Button variant="ghost" size="icon" onClick={() => setDeleteConfirm(ch.id)}>
              <Trash2 className="h-4 w-4" />
            </Button>
          </div>
        </div>
      )}
    </div>
  )

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>Notifications</CardTitle>
            <CardDescription>Per-project Slack and Telegram channel alerts</CardDescription>
          </div>
          <Button onClick={handleStartCreate} disabled={isCreating}>
            <Plus className="h-4 w-4 mr-2" />
            New Channel
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {isLoading && (
            <div className="text-center py-8 text-muted-foreground">Loading channels...</div>
          )}
          {error && (
            <div className="text-center py-8 text-destructive">
              Error: {(error as Error).message}
            </div>
          )}

          {isCreating && (
            <NotificationChannelForm
              formData={formData}
              setFormData={setFormData}
              onCancel={handleCancel}
              onSave={handleSaveCreate}
              mutation={createMutation}
              isCreate
            />
          )}

          {!isLoading && !error && channels.length === 0 && !isCreating && (
            <div className="text-center py-8 text-muted-foreground">
              No notification channels configured. Create one to get started.
            </div>
          )}

          {channels.map(renderChannelRow)}
        </div>
      </CardContent>
    </Card>
  )
}
