import { notifications } from '@mantine/notifications'
import { ApiError } from '../api/client'

export function notifySuccess(message: string, title = 'Done') {
  notifications.show({ title, message, color: 'green' })
}

export function notifyError(err: unknown, title = 'Failed') {
  const message = err instanceof ApiError ? err.message : err instanceof Error ? err.message : String(err)
  notifications.show({ title, message, color: 'red' })
}
