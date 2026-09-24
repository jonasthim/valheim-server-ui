import { createElement } from 'react'
import { notifications } from '@mantine/notifications'
import { IconAlertTriangle, IconCheck, IconInfoCircle, IconX } from '@tabler/icons-react'
import { ApiError } from '../api/client'

export type NotifyTone = 'success' | 'error' | 'warning' | 'info'

// One toast style: 16px icon in a hue disc, neutral surface, 3.5 s. Icon colour
// per hue for ≥ 3:1 against the disc (white fails on moss/straw/frost).
const TONES: Record<NotifyTone, { color: string; icon: typeof IconCheck; iconColor: string }> = {
  success: { color: 'moss', icon: IconCheck, iconColor: '#18181B' },
  error: { color: 'blood', icon: IconX, iconColor: '#FFFFFF' },
  warning: { color: 'straw', icon: IconAlertTriangle, iconColor: '#18181B' },
  info: { color: 'frost', icon: IconInfoCircle, iconColor: '#18181B' },
}

export function notify(tone: NotifyTone, message: string, title?: string) {
  const { color, icon, iconColor } = TONES[tone]
  notifications.show({ title, message, color, icon: createElement(icon, { size: 16, color: iconColor }), autoClose: 3500 })
}

export function notifySuccess(message: string, title = 'Done') {
  notify('success', message, title)
}

export function notifyError(err: unknown, title = 'Failed') {
  const message = err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err)
  notify('error', message, title)
}

export function notifyWarning(message: string, title?: string) {
  notify('warning', message, title)
}

export function notifyInfo(message: string, title?: string) {
  notify('info', message, title)
}
