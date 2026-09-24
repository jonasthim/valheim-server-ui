// Full-page 403: RequireRole (auth/guards.tsx) renders this instead of a
// silent redirect to `/` when the signed-in user's role is below the route's
// minimum.
import { Button, Center, Group, Stack, Text, Title } from '@mantine/core'
import { useDocumentTitle } from '@mantine/hooks'
import { Link, useNavigate } from 'react-router-dom'
import type { Role } from '../../api/types'
import { pageTitle } from '../../lib/title'

export function ForbiddenPage({ required }: { required?: Role }) {
  useDocumentTitle(pageTitle('Access denied'))
  const navigate = useNavigate()
  return (
    <Center mih="60vh">
      <Stack align="center" gap="sm" ta="center" maw={420}>
        <Text c="dimmed" fw={600}>
          403
        </Text>
        <Title order={1} size="h2">
          You don't have access to this page
        </Title>
        <Text size="sm" c="dimmed">
          This page requires the {required ?? 'admin'} role. Ask an administrator if you need access.
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
