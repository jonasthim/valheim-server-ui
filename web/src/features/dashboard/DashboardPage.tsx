import { ActionIcon, Button, SimpleGrid, Skeleton, Stack, Tooltip } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { IconDatabase, IconPlus, IconRefresh, IconRocket, IconServer2, IconUsers } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { api } from '../../api/client'
import type { Instance } from '../../api/types'
import { fmtBytes } from '../../lib/format'
import { EmptyState, PageHeader, StatTile } from '../../ui'
import { useCheckAppUpdate, useSystemInfo } from '../system'
import { SystemStrip } from './SystemStrip'
import { InstanceCard } from './InstanceCard'

export function DashboardPage() {
  const { hasRole } = useAuth()
  const instancesQuery = useQuery({
    queryKey: ['instances', 'list'],
    queryFn: () => api.get<{ instances: Instance[] }>('/instances').then((r) => r.instances),
    refetchInterval: 15_000,
  })
  const system = useSystemInfo()
  const checkAppUpdate = useCheckAppUpdate()

  const instances = instancesQuery.data ?? []
  const running = instances.filter((i) => i.status.state === 'running').length
  const stopped = instances.length - running
  const playersOnline = instances.reduce((sum, i) => sum + i.status.players_online, 0)
  const info = system.data
  const updateAvailable = info?.app_update?.update_available ?? false

  return (
    <Stack gap="xl">
      <PageHeader
        eyebrow="Overview"
        title="Dashboard"
        description="Your Valheim servers at a glance"
        actions={
          hasRole('admin') && (
            <Button component={Link} to="/instances/new" leftSection={<IconPlus size={16} />}>
              New instance
            </Button>
          )
        }
      />

      {system.isLoading ? (
        <SimpleGrid cols={{ base: 2, md: 4 }}>
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} height={92} radius="lg" />
          ))}
        </SimpleGrid>
      ) : (
        <SimpleGrid cols={{ base: 2, md: 4 }}>
          <StatTile
            label="Servers"
            value={`${running} / ${instances.length}`}
            hint={`${stopped} stopped`}
            icon={<IconServer2 size={16} />}
          />
          <StatTile
            label="Players online"
            value={playersOnline}
            icon={<IconUsers size={16} />}
            accent="var(--vh-moss)"
          />
          <StatTile
            label="Disk free"
            value={info ? fmtBytes(info.disk_free_bytes) : '-'}
            hint={info ? `of ${fmtBytes(info.disk_total_bytes)}` : undefined}
            icon={<IconDatabase size={16} />}
          />
          <StatTile
            label="Manager"
            value={info ? `v${info.version.replace(/^v/, '')}` : '-'}
            hint={updateAvailable ? `v${info?.app_update?.latest_version} available` : 'Up to date'}
            accent={updateAvailable ? 'var(--vh-ember)' : undefined}
            icon={
              hasRole('admin') ? (
                <Tooltip label="Check for a new Valheim Server UI release now">
                  <ActionIcon
                    variant="subtle"
                    color="gray"
                    size="sm"
                    loading={checkAppUpdate.isPending}
                    onClick={() => checkAppUpdate.mutate()}
                    aria-label="Check for application update"
                  >
                    <IconRefresh size={14} />
                  </ActionIcon>
                </Tooltip>
              ) : (
                <IconRocket size={16} />
              )
            }
          />
        </SimpleGrid>
      )}

      <SystemStrip instances={instances} />

      <Stack gap="md">
        {instancesQuery.isLoading && (
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }}>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} height={220} radius="lg" />
            ))}
          </SimpleGrid>
        )}

        {!instancesQuery.isLoading && instances.length === 0 && (
          <EmptyState
            icon={<IconServer2 size={22} />}
            title="No instances yet"
            description="Create your first Valheim server to get started."
            action={
              hasRole('admin') && (
                <Button component={Link} to="/instances/new" leftSection={<IconPlus size={16} />} mt="xs">
                  Create your first instance
                </Button>
              )
            }
          />
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
