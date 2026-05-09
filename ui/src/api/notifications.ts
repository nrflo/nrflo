import { apiGet, apiPost, apiPatch, apiDelete } from './client'
import type { NotificationChannel, NotificationDelivery } from '@/types/notifications'

export interface CreateNotificationChannelRequest {
  name: string
  kind: string
  enabled: boolean
  config: string
  event_types: string[]
}

export interface UpdateNotificationChannelRequest {
  name?: string
  enabled?: boolean
  config?: string
  event_types?: string[]
}

export async function listNotificationChannels(workflowId: string): Promise<NotificationChannel[]> {
  return apiGet<NotificationChannel[]>(`/api/v1/workflows/${workflowId}/notification-channels`)
}

export async function getNotificationChannel(workflowId: string, id: string): Promise<NotificationChannel> {
  return apiGet<NotificationChannel>(`/api/v1/workflows/${workflowId}/notification-channels/${id}`)
}

export async function createNotificationChannel(workflowId: string, req: CreateNotificationChannelRequest): Promise<NotificationChannel> {
  return apiPost<NotificationChannel>(`/api/v1/workflows/${workflowId}/notification-channels`, req)
}

export async function updateNotificationChannel(workflowId: string, id: string, req: UpdateNotificationChannelRequest): Promise<NotificationChannel> {
  return apiPatch<NotificationChannel>(`/api/v1/workflows/${workflowId}/notification-channels/${id}`, req)
}

export async function deleteNotificationChannel(workflowId: string, id: string): Promise<void> {
  return apiDelete(`/api/v1/workflows/${workflowId}/notification-channels/${id}`)
}

export async function testNotificationChannel(workflowId: string, id: string): Promise<void> {
  return apiPost(`/api/v1/workflows/${workflowId}/notification-channels/${id}/test`, {})
}

export async function listNotificationDeliveries({ workflowId, channelId, limit = 20 }: { workflowId: string; channelId: string; limit?: number }): Promise<NotificationDelivery[]> {
  return apiGet<NotificationDelivery[]>(`/api/v1/workflows/${workflowId}/notification-deliveries?channel_id=${channelId}&limit=${limit}`)
}
