import { Alert, Button, Group } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import { ApiError } from '../api/client'

/**
 * Inline "this data failed to load" state for a page whose query errored, so a
 * failed GET shows a real message instead of a stuck spinner or a misleading
 * empty state ("No instances yet", "0 servers"). A 404 gets a not-found title;
 * everything else is a generic load failure with the server's message.
 */
export function LoadError({
  error,
  title,
  onRetry,
}: {
  error: unknown
  title?: string
  onRetry?: () => void
}) {
  const notFound = error instanceof ApiError && error.status === 404
  const heading = title ?? (notFound ? 'Not found' : 'Could not load')
  const message = error instanceof Error ? error.message : 'Something went wrong loading this data.'
  return (
    <Alert color={notFound ? 'gray' : 'red'} icon={<IconAlertTriangle size={18} />} title={heading} my="md">
      {message}
      {onRetry && (
        <Group mt="sm">
          <Button size="xs" variant="light" onClick={onRetry}>
            Try again
          </Button>
        </Group>
      )}
    </Alert>
  )
}
