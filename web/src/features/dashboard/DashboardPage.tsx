import { ActionIcon, Button, SimpleGrid, Skeleton, Stack, Tooltip } from '@mantine/core'
import { useDocumentTitle } from '@mantine/hooks'
import { Link } from 'react-router-dom'
import { IconCpu, IconDatabase, IconDeviceSdCard, IconPlus, IconRefresh, IconRocket, IconServer2, IconUsers } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtBytes, fmtPercent } from '../../lib/format'
import { pageTitle } from '../../lib/title'
import { EmptyState, LoadError, PageHeader, StatStrip } from '../../ui'
import { useCheckAppUpdate, useSystemInfo } from '../system'
import { useInstances } from '../instances'
import { useSystemMetrics } from '../instances/useMetrics'
import { SystemStrip } from './SystemStrip'
import { InstanceCard } from './InstanceCard'

export function DashboardPage() {
  useDocumentTitle(pageTitle('Dashboard'))
  const { hasRole } = useAuth()
  const instancesQuery = useInstances()
  const system = useSystemInfo()
  const checkAppUpdate = useCheckAppUpdate()
  const metrics = useSystemMetrics('24h')

  const instances = instancesQuery.data ?? []
  const running = instances.filter((i) => i.status.state === 'running').length
  const stopped = instances.length - running
  const playersOnline = instances.reduce((sum, i) => sum + i.status.players_online, 0)
  const info = system.data
  const host = info?.host
  const updateAvailable = info?.app_update?.update_available ?? false

  return (
    <Stack gap="lg">
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

      <StatStrip
        minCellWidth={150}
        loading={system.isLoading}
        items={[
          {
            key: 'servers',
            label: 'Servers',
            value: `${running} / ${instances.length}`,
            hint: `${stopped} stopped`,
            icon: <IconServer2 size={16} />,
          },
          {
            key: 'players',
            label: 'Players online',
            value: playersOnline,
            icon: <IconUsers size={16} />,
            tone: playersOnline > 0 ? 'success' : 'default',
          },
          {
            key: 'cpu',
            label: 'CPU',
            value: host ? fmtPercent(host.cpu_percent) : '—',
            hint: host ? `load ${host.load_avg_1.toFixed(2)}, ${host.cpu_count} cores` : undefined,
            icon: <IconCpu size={16} />,
            tone: host && host.cpu_percent >= 85 ? 'danger' : 'default',
            spark: metrics.data?.cpu,
            sparkFormat: fmtPercent,
          },
          {
            key: 'mem',
            label: 'Memory',
            value: host ? fmtBytes(host.mem_used_bytes) : '—',
            hint: host ? `of ${fmtBytes(host.mem_total_bytes)} (${fmtPercent((host.mem_used_bytes / host.mem_total_bytes) * 100)})` : undefined,
            icon: <IconDeviceSdCard size={16} />,
            tone: host && host.mem_used_bytes / host.mem_total_bytes >= 0.9 ? 'danger' : 'default',
            spark: metrics.data?.mem,
            sparkFormat: fmtBytes,
          },
          {
            key: 'disk',
            label: 'Disk free',
            value: info ? fmtBytes(info.disk_free_bytes) : '—',
            hint: info ? `of ${fmtBytes(info.disk_total_bytes)}` : undefined,
            icon: <IconDatabase size={16} />,
          },
          {
            key: 'manager',
            label: 'Manager',
            value: info ? `v${info.version.replace(/^v/, '')}` : '—',
            hint: updateAvailable ? `v${info?.app_update?.latest_version} available` : 'Up to date',
            tone: updateAvailable ? 'accent' : 'default',
            icon: hasRole('admin') ? undefined : <IconRocket size={16} />,
            action: hasRole('admin') ? (
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
            ) : undefined,
          },
        ]}
      />

      <SystemStrip instances={instances} />

      <Stack gap="md">
        {instancesQuery.isLoading && (
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }} spacing="md">
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} height={180} radius="md" />
            ))}
          </SimpleGrid>
        )}

        {instancesQuery.isError && (
          <LoadError
            error={instancesQuery.error}
            title="Could not load your instances"
            onRetry={() => instancesQuery.refetch()}
          />
        )}

        {!instancesQuery.isLoading && !instancesQuery.isError && instances.length === 0 && (
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
          <SimpleGrid cols={{ base: 1, sm: 2, lg: 3 }} spacing="md">
            {instances.map((instance) => (
              <InstanceCard key={instance.id} instance={instance} />
            ))}
          </SimpleGrid>
        )}
      </Stack>
    </Stack>
  )
}
