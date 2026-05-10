import { useQuery, useMutation } from '@tanstack/react-query'
import { Check, X, Send } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/Button'
import { Input } from '@/components/ui/Input'
import { Dropdown } from '@/components/ui/Dropdown'
import { Toggle } from '@/components/ui/Toggle'
import { Badge } from '@/components/ui/Badge'
import { Table, TableHeader, TableBody, TableRow, TableHead, TableCell } from '@/components/ui/Table'
import { testNotificationChannel, listNotificationDeliveries } from '@/api/notifications'
import type { NotificationChannel, NotificationEventType } from '@/types/notifications'

const kindOptions = [
  { value: 'slack', label: 'Slack' },
  { value: 'telegram', label: 'Telegram' },
]

const ALL_EVENT_TYPES: { value: NotificationEventType; label: string }[] = [
  { value: 'workflow.completed', label: 'Workflow Completed' },
  { value: 'workflow.failed', label: 'Workflow Failed' },
  { value: 'agent.completed', label: 'Agent Completed' },
  { value: 'agent.context_saving', label: 'Agent Context Saving' },
  { value: 'agent.stall_restart', label: 'Agent Stall Restart' },
]

export interface ChannelFormData {
  name: string
  kind: string
  enabled: boolean
  webhookUrl: string
  botToken: string
  chatId: string
  eventTypes: NotificationEventType[]
}

export function emptyChannelForm(): ChannelFormData {
  return { name: '', kind: 'slack', enabled: true, webhookUrl: '', botToken: '', chatId: '', eventTypes: [] }
}

export function channelToFormData(ch: NotificationChannel): ChannelFormData {
  let webhookUrl = ''
  let botToken = ''
  let chatId = ''
  try {
    const cfg = JSON.parse(ch.config) as Record<string, string>
    webhookUrl = cfg['webhook_url'] ?? ''
    botToken = cfg['bot_token'] ?? ''
    chatId = cfg['chat_id'] ?? ''
  } catch { /* ignore parse error */ }
  return {
    name: ch.name,
    kind: ch.kind,
    enabled: ch.enabled,
    webhookUrl,
    botToken,
    chatId,
    eventTypes: ch.event_types ?? [],
  }
}

export function buildConfig(formData: ChannelFormData): Record<string, unknown> {
  if (formData.kind === 'slack') {
    return { webhook_url: formData.webhookUrl }
  }
  return { bot_token: formData.botToken, chat_id: formData.chatId }
}

const deliveryKeys = {
  list: (workflowId: string, channelId: string) => ['notification-deliveries', workflowId, channelId] as const,
}

export function NotificationChannelForm({
  workflowId,
  formData,
  setFormData,
  onCancel,
  onSave,
  mutation,
  isCreate,
  editingChannel,
}: {
  workflowId: string
  formData: ChannelFormData
  setFormData: (data: ChannelFormData) => void
  onCancel: () => void
  onSave: () => void
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  mutation: { isPending: boolean; isError: boolean; error: any }
  isCreate?: boolean
  editingChannel?: NotificationChannel
}) {
  const testMutation = useMutation({
    mutationFn: () => testNotificationChannel(workflowId, editingChannel!.id),
    onSuccess: () => toast.success('Test notification sent'),
    onError: (err: Error) => toast.error(`Test failed: ${err.message}`),
  })

  const { data: deliveries = [] } = useQuery({
    queryKey: deliveryKeys.list(workflowId, editingChannel?.id ?? ''),
    queryFn: () => listNotificationDeliveries({ workflowId, channelId: editingChannel!.id, limit: 20 }),
    enabled: !!editingChannel,
  })

  const hasOneEventType = formData.eventTypes.length > 0
  const isDisabled = mutation.isPending || (!hasOneEventType)

  const toggleEventType = (et: NotificationEventType) => {
    const current = formData.eventTypes
    const next = current.includes(et) ? current.filter((e) => e !== et) : [...current, et]
    setFormData({ ...formData, eventTypes: next })
  }

  return (
    <div className={`space-y-3 ${isCreate ? 'border border-primary rounded-lg p-4 bg-muted/30' : ''}`}>
      <div className="grid grid-cols-2 gap-3">
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            Name {isCreate && <span className="text-destructive">*</span>}
          </label>
          <Input
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            placeholder="e.g. #alerts"
          />
        </div>
        <div>
          <label className="text-sm font-medium text-muted-foreground">Kind</label>
          {isCreate ? (
            <Dropdown
              value={formData.kind}
              onChange={(val) => setFormData({ ...formData, kind: val, webhookUrl: '', botToken: '', chatId: '' })}
              options={kindOptions}
            />
          ) : (
            <Input value={formData.kind} disabled className="bg-muted" />
          )}
        </div>
      </div>

      <div>
        <Toggle
          checked={formData.enabled}
          onChange={(val) => setFormData({ ...formData, enabled: val })}
          label="Enabled"
        />
      </div>

      {formData.kind === 'slack' && (
        <div>
          <label className="text-sm font-medium text-muted-foreground">
            Webhook URL {isCreate && <span className="text-destructive">*</span>}
          </label>
          <Input
            value={formData.webhookUrl}
            onChange={(e) => setFormData({ ...formData, webhookUrl: e.target.value })}
            placeholder="https://hooks.slack.com/services/..."
            type="password"
          />
        </div>
      )}

      {formData.kind === 'telegram' && (
        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="text-sm font-medium text-muted-foreground">
              Bot Token {isCreate && <span className="text-destructive">*</span>}
            </label>
            <Input
              value={formData.botToken}
              onChange={(e) => setFormData({ ...formData, botToken: e.target.value })}
              placeholder="123456:ABC-DEF..."
              type="password"
            />
          </div>
          <div>
            <label className="text-sm font-medium text-muted-foreground">
              Chat ID {isCreate && <span className="text-destructive">*</span>}
            </label>
            <Input
              value={formData.chatId}
              onChange={(e) => setFormData({ ...formData, chatId: e.target.value })}
              placeholder="-1001234567890"
            />
          </div>
        </div>
      )}

      <div>
        <label className="text-sm font-medium text-muted-foreground block mb-1.5">
          Event Types <span className="text-destructive">*</span>
        </label>
        <div className="flex flex-wrap gap-2">
          {ALL_EVENT_TYPES.map(({ value, label }) => {
            const selected = formData.eventTypes.includes(value)
            return (
              <button
                key={value}
                type="button"
                onClick={() => toggleEventType(value)}
                className={`text-xs px-2.5 py-1 rounded-full border transition-colors ${
                  selected
                    ? 'border-primary bg-primary/10 text-primary'
                    : 'border-border text-muted-foreground hover:border-primary/50'
                }`}
              >
                {label}
              </button>
            )
          })}
        </div>
        {!hasOneEventType && (
          <p className="text-xs text-destructive mt-1">Select at least one event type.</p>
        )}
      </div>

      <div className="flex gap-2 justify-end">
        <Button variant="ghost" onClick={onCancel}>
          {isCreate ? 'Cancel' : <><X className="h-4 w-4 mr-1" />Cancel</>}
        </Button>
        {editingChannel && (
          <Button
            variant="outline"
            onClick={() => testMutation.mutate()}
            disabled={testMutation.isPending}
          >
            <Send className="h-4 w-4 mr-1" />
            {testMutation.isPending ? 'Sending...' : 'Send Test'}
          </Button>
        )}
        <Button onClick={onSave} disabled={isDisabled || !formData.name.trim()}>
          {isCreate ? (
            mutation.isPending ? 'Creating...' : 'Create'
          ) : (
            <>{mutation.isPending ? 'Saving...' : <><Check className="h-4 w-4 mr-1" />Save</>}</>
          )}
        </Button>
      </div>

      {mutation.isError && (
        <p className="text-sm text-destructive">
          Error: {mutation.error?.message}
        </p>
      )}

      {editingChannel && deliveries.length > 0 && (
        <div className="pt-2 border-t border-border">
          <div className="text-sm font-semibold text-muted-foreground mb-2">Recent Deliveries</div>
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Time</TableHead>
                <TableHead>Event</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Attempts</TableHead>
                <TableHead>Error</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {deliveries.map((d) => (
                <TableRow key={d.id}>
                  <TableCell className="text-xs">{new Date(d.created_at).toLocaleString()}</TableCell>
                  <TableCell className="text-xs font-mono">{d.event_type}</TableCell>
                  <TableCell>
                    <Badge
                      variant={d.status === 'delivered' ? 'success' : d.status === 'failed' ? 'destructive' : 'secondary'}
                      className="text-xs"
                    >
                      {d.status}
                    </Badge>
                  </TableCell>
                  <TableCell className="text-xs">{d.attempts}</TableCell>
                  <TableCell className="text-xs text-muted-foreground truncate max-w-[200px]">
                    {d.last_error || '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
    </div>
  )
}
