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

export async function listNotificationChannels(): Promise<NotificationChannel[]> {
  return apiGet<NotificationChannel[]>('/api/v1/notification-channels')
}

export async function getNotificationChannel(id: number): Promise<NotificationChannel> {
  return apiGet<NotificationChannel>(`/api/v1/notification-channels/${id}`)
}

export async function createNotificationChannel(req: CreateNotificationChannelRequest): Promise<NotificationChannel> {
  return apiPost<NotificationChannel>('/api/v1/notification-channels', req)
}

export async function updateNotificationChannel(id: number, req: UpdateNotificationChannelRequest): Promise<NotificationChannel> {
  return apiPatch<NotificationChannel>(`/api/v1/notification-channels/${id}`, req)
}

export async function deleteNotificationChannel(id: number): Promise<void> {
  return apiDelete(`/api/v1/notification-channels/${id}`)
}

export async function testNotificationChannel(id: number): Promise<void> {
  return apiPost(`/api/v1/notification-channels/${id}/test`, {})
}

export async function listNotificationDeliveries({ channelId, limit = 20 }: { channelId: number; limit?: number }): Promise<NotificationDelivery[]> {
  return apiGet<NotificationDelivery[]>(`/api/v1/notification-deliveries?channel_id=${channelId}&limit=${limit}`)
}
