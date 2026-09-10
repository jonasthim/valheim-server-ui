import { Component, type ErrorInfo, type ReactNode } from 'react'
import { Alert, Button, Code, Stack, Text } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'

interface Props {
  children: ReactNode
}

interface State {
  error: Error | null
}

/**
 * Catches render/runtime errors in its subtree and shows a message instead of
 * a blank page. Without this, one bad config or unexpected value unmounts the
 * whole app.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    // Surface it for debugging; the boundary keeps the app usable.
    console.error('UI error boundary caught:', error, info.componentStack)
  }

  render() {
    const { error } = this.state
    if (!error) return this.props.children
    return (
      <Alert color="red" icon={<IconAlertTriangle size={18} />} title="Something went wrong on this screen" my="md">
        <Stack gap="sm" align="flex-start">
          <Text size="sm">
            This view hit an error and stopped rendering. The rest of the app is fine — go back or reload. If it
            keeps happening, this message has the detail to report.
          </Text>
          {error.message && (
            <Code block style={{ maxWidth: '100%', overflowX: 'auto' }}>
              {error.message}
            </Code>
          )}
          <Button size="xs" variant="light" onClick={() => this.setState({ error: null })}>
            Try again
          </Button>
        </Stack>
      </Alert>
    )
  }
}
