// Full-page 404: the App.tsx catch-all route, an unknown instance tab, and a
// 404 from the instance detail fetch (InstancePage) all render this instead
// of a silent redirect to `/`.
import { Button, Center, Group, Stack, Text, Title } from '@mantine/core'
import { useDocumentTitle } from '@mantine/hooks'
import { Link, useNavigate } from 'react-router-dom'
import { pageTitle } from '../../lib/title'

export function NotFoundPage() {
  useDocumentTitle(pageTitle('Page not found'))
  const navigate = useNavigate()
  return (
    <Center mih="60vh">
      <Stack align="center" gap="sm" ta="center" maw={420}>
        <Text c="dimmed" fw={600}>
          404
        </Text>
        <Title order={1} size="h2">
          Page not found
        </Title>
        <Text size="sm" c="dimmed">
          The page you are looking for does not exist or has moved.
        </Text>
        <Group>
          <Button component={Link} to="/">
            Go to dashboard
          </Button>
          <Button variant="default" onClick={() => navigate(-1)}>
            Go back
          </Button>
        </Group>
      </Stack>
    </Center>
  )
}
