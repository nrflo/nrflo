import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Plus, Pencil, Trash2, Bot } from 'lucide-react'
import { Button } from '@/components/ui/Button'
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from '@/components/ui/Card'
import {
  listSystemAgentDefs,
  createSystemAgentDef,
  updateSystemAgentDef,
  deleteSystemAgentDef,
  type SystemAgentDef,
  type CreateSystemAgentDefRequest,
  type UpdateSystemAgentDefRequest,
} from '@/api/systemAgentDefs'
import { AgentForm, emptyAgentForm, parseOptionalInt, type AgentFormData } from './AgentForm'

const systemAgentKeys = {
  all: ['system-agents'] as const,
  list: () => [...systemAgentKeys.all, 'list'] as const,
}

function agentToFormData(agent: SystemAgentDef): AgentFormData {
  return {
    id: agent.id,
    model: agent.model,
    timeout: String(agent.timeout),
    prompt: agent.prompt,
    restart_threshold: agent.restart_threshold != null ? String(agent.restart_threshold) : '',
    max_fail_restarts: agent.max_fail_restarts != null ? String(agent.max_fail_restarts) : '',
    stall_start_timeout_sec: agent.stall_start_timeout_sec != null ? String(agent.stall_start_timeout_sec) : '',
    stall_running_timeout_sec: agent.stall_running_timeout_sec != null ? String(agent.stall_running_timeout_sec) : '',
  }
}

export function SystemAgentsSection() {
  const queryClient = useQueryClient()

  const [isCreating, setIsCreating] = useState(false)
  const [editingId, setEditingId] = useState<string | null>(null)
  const [formData, setFormData] = useState<AgentFormData>(emptyAgentForm)
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  const { data: agents = [], isLoading, error } = useQuery({
    queryKey: systemAgentKeys.list(),
    queryFn: listSystemAgentDefs,
  })

  const createMutation = useMutation({
    mutationFn: (data: CreateSystemAgentDefRequest) => createSystemAgentDef(data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: systemAgentKeys.list() })
      setIsCreating(false)
      setFormData(emptyAgentForm)
    },
  })

  const updateMutation = useMutation({
    mutationFn: ({ id, data }: { id: string; data: UpdateSystemAgentDefRequest }) =>
      updateSystemAgentDef(id, data),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: systemAgentKeys.list() })
      setEditingId(null)
      setFormData(emptyAgentForm)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: (id: string) => deleteSystemAgentDef(id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: systemAgentKeys.list() })
      setDeleteConfirm(null)
    },
  })

  const handleStartCreate = () => {
    setIsCreating(true)
    setEditingId(null)
    setFormData(emptyAgentForm)
  }

  const handleStartEdit = (agent: SystemAgentDef) => {
    setEditingId(agent.id)
    setIsCreating(false)
    setFormData(agentToFormData(agent))
  }

  const handleCancel = () => {
    setIsCreating(false)
    setEditingId(null)
    setFormData(emptyAgentForm)
  }

  const handleSaveCreate = () => {
    if (!formData.id.trim() || !formData.prompt.trim()) return
    createMutation.mutate({
      id: formData.id.trim(),
      model: formData.model,
      timeout: parseInt(formData.timeout, 10) || 30,
      prompt: formData.prompt,
      restart_threshold: parseOptionalInt(formData.restart_threshold),
      max_fail_restarts: parseOptionalInt(formData.max_fail_restarts),
      stall_start_timeout_sec: parseOptionalInt(formData.stall_start_timeout_sec),
      stall_running_timeout_sec: parseOptionalInt(formData.stall_running_timeout_sec),
    })
  }

  const handleSaveEdit = () => {
    if (!editingId) return
    updateMutation.mutate({
      id: editingId,
      data: {
        model: formData.model,
        timeout: parseInt(formData.timeout, 10) || 30,
        prompt: formData.prompt,
        restart_threshold: parseOptionalInt(formData.restart_threshold),
        max_fail_restarts: parseOptionalInt(formData.max_fail_restarts),
        stall_start_timeout_sec: parseOptionalInt(formData.stall_start_timeout_sec),
        stall_running_timeout_sec: parseOptionalInt(formData.stall_running_timeout_sec),
      },
    })
  }

  return (
    <Card>
      <CardHeader>
        <div className="flex items-center justify-between">
          <div>
            <CardTitle>System Agents</CardTitle>
            <CardDescription>Global agent definitions (e.g., conflict-resolver)</CardDescription>
          </div>
          <Button onClick={handleStartCreate} disabled={isCreating}>
            <Plus className="h-4 w-4 mr-2" />
            New System Agent
          </Button>
        </div>
      </CardHeader>
      <CardContent>
        <div className="space-y-3">
          {isLoading && (
            <div className="text-center py-8 text-muted-foreground">Loading system agents...</div>
          )}
          {error && (
            <div className="text-center py-8 text-destructive">
              Error: {(error as Error).message}
            </div>
          )}

          {isCreating && (
            <AgentForm
              formData={formData}
              setFormData={setFormData}
              onCancel={handleCancel}
              onSave={handleSaveCreate}
              mutation={createMutation}
              isCreate
            />
          )}

          {!isLoading && !error && agents.length === 0 && !isCreating && (
            <div className="text-center py-8 text-muted-foreground">
              No system agents defined. Create one to get started.
            </div>
          )}

          {agents.map((agent) => (
            <div key={agent.id} className="border rounded-lg p-4">
              {editingId === agent.id ? (
                <AgentForm
                  formData={formData}
                  setFormData={setFormData}
                  onCancel={handleCancel}
                  onSave={handleSaveEdit}
                  mutation={updateMutation}
                />
              ) : deleteConfirm === agent.id ? (
                <div className="flex items-center justify-between">
                  <div className="text-sm">
                    Are you sure you want to delete{' '}
                    <span className="font-semibold">{agent.id}</span>?
                  </div>
                  <div className="flex gap-2">
                    <Button variant="ghost" onClick={() => setDeleteConfirm(null)}>
                      Cancel
                    </Button>
                    <Button
                      variant="destructive"
                      onClick={() => deleteMutation.mutate(agent.id)}
                      disabled={deleteMutation.isPending}
                    >
                      {deleteMutation.isPending ? 'Deleting...' : 'Delete'}
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-3 min-w-0">
                    <Bot className="h-5 w-5 text-muted-foreground shrink-0" />
                    <div className="min-w-0">
                      <div className="font-medium">{agent.id}</div>
                      <div className="text-sm text-muted-foreground">
                        {[
                          `Model: ${agent.model}`,
                          `Timeout: ${agent.timeout}m`,
                        ].join(' | ')}
                      </div>
                    </div>
                  </div>
                  <div className="flex gap-1">
                    <Button variant="ghost" size="icon" onClick={() => handleStartEdit(agent)}>
                      <Pencil className="h-4 w-4" />
                    </Button>
                    <Button variant="ghost" size="icon" onClick={() => setDeleteConfirm(agent.id)}>
                      <Trash2 className="h-4 w-4" />
                    </Button>
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      </CardContent>
    </Card>
  )
}
