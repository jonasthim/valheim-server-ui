import { Alert, Button, Code, Stack, Text } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'
import { useRouteError } from 'react-router-dom'

/**
 * Router-level errorElement: catches errors the data router surfaces outside
 * any page's own ErrorBoundary (e.g. LoginPage/SetupPage, or a render error
 * before Shell mounts) so the app never blanks to a white page. Mirrors
 * ErrorBoundary's fallback markup; "Try again" reloads since there is no
 * component state to reset here.
 */
export function RouteErrorPage() {
  const error = useRouteError()
  return (
    <Alert color="red" icon={<IconAlertTriangle size={16} />} title="Something went wrong on this screen" my="md">
      <Stack gap="sm" align="flex-start">
        <Text size="sm">
          This view hit an error and stopped rendering. The rest of the app is fine — go back or reload. If it
          keeps happening, this message has the detail to report.
        </Text>
        <Code block style={{ maxWidth: '100%', overflowX: 'auto' }}>
          {error instanceof Error ? error.message : String(error)}
        </Code>
        <Button size="xs" variant="light" onClick={() => window.location.reload()}>
          Try again
        </Button>
      </Stack>
    </Alert>
  )
}
