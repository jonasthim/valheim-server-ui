import { Button, Center, Group, SimpleGrid, Skeleton, Stack, Text, Title } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { IconServer2, IconPlus } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { api } from '../../api/client'
import type { Instance } from '../../api/types'
import { SystemStrip } from './SystemStrip'
import { InstanceCard } from './InstanceCard'

export function DashboardPage() {
  const { hasRole } = useAuth()
  const instancesQuery = useQuery({
    queryKey: ['instances', 'list'],
    queryFn: () => api.get<{ instances: Instance[] }>('/instances').then((r) => r.instances),
    refetchInterval: 15_000,
  })

  const instances = instancesQuery.data ?? []

  return (
    <Stack>
      <SystemStrip />

      <Stack gap="md">
        <Group justify="space-between">
          <Title order={2}>Instances</Title>
          {hasRole('admin') && (
            <Button component={Link} to="/instances/new" leftSection={<IconPlus size={16} />}>
              New instance
            </Button>
          )}
        </Group>

        {instancesQuery.isLoading && (
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} height={220} radius="md" />
            ))}
          </SimpleGrid>
        )}

        {!instancesQuery.isLoading && instances.length === 0 && (
          <Center py="xl">
            <Stack align="center" gap="sm">
              <IconServer2 size={40} opacity={0.5} />
              <Text c="dimmed">No instances yet.</Text>
              {hasRole('admin') && (
                <Button component={Link} to="/instances/new" leftSection={<IconPlus size={16} />}>
                  Create your first instance
                </Button>
              )}
            </Stack>
          </Center>
        )}

        {instances.length > 0 && (
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
            {instances.map((instance) => (
              <InstanceCard key={instance.id} instance={instance} />
            ))}
          </SimpleGrid>
        )}
      </Stack>
    </Stack>
  )
}
