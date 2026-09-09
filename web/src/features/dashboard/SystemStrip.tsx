// Compact system info strip shown above the instance grid: manager health,
// the manager's own update banner (WP-32), and a rollup of instances with a
// game update available.
import { ActionIcon, Alert, Anchor, Group, Paper, Skeleton, Stack, Text, Tooltip } from '@mantine/core'
import { IconAlertTriangle, IconDownload, IconRefresh } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import { useAuth } from '../../auth/useAuth'
import type { Instance } from '../../api/types'
import { fmtBytes } from '../../lib/format'
import { AppUpdateBanner, useCheckAppUpdate, useSystemInfo } from '../system'

export function SystemStrip({ instances }: { instances: Instance[] }) {
  const { hasRole } = useAuth()
  const query = useSystemInfo()
  const checkAppUpdate = useCheckAppUpdate()

  if (query.isLoading) return <Skeleton height={48} />
  if (!query.data) return null

  const info = query.data
  const instancesWithUpdate = instances.filter((i) => i.status.update_available)
  const firstUpdateInstance = instancesWithUpdate[0]

  return (
    <Stack gap="sm">
      <Paper withBorder p="sm">
        <Group justify="space-between" wrap="wrap" gap="md">
          <Group gap="lg">
            <Text size="sm" c="dimmed">
              Version <Text span c="var(--mantine-color-text)">{info.version}</Text>
            </Text>
            <Text size="sm" c="dimmed">
              Disk free <Text span c="var(--mantine-color-text)">{fmtBytes(info.disk_free_bytes)}</Text> of {fmtBytes(info.disk_total_bytes)}
            </Text>
            <Text size="sm" c="dimmed">
              Supervisor <Text span c="var(--mantine-color-text)">{info.supervisor}</Text>
            </Text>
          </Group>
          <Group gap="sm" wrap="wrap">
            {hasRole('admin') && (
              <Tooltip label="Check for a new Valheim Server UI release now">
                <ActionIcon
                  variant="subtle"
                  loading={checkAppUpdate.isPending}
                  onClick={() => checkAppUpdate.mutate()}
                  aria-label="Check for application update"
                >
                  <IconRefresh size={16} />
                </ActionIcon>
              </Tooltip>
            )}
            {!info.steamcmd_installed && (
              <Alert color="red" icon={<IconAlertTriangle size={16} />} py={4} px="sm">
                SteamCMD is not installed. Run the installer (see docs/RUNBOOK.md) before installing any instance.
              </Alert>
            )}
          </Group>
        </Group>
      </Paper>

      <AppUpdateBanner appUpdate={info.app_update} />

      {firstUpdateInstance && (
        <Alert color="blue" variant="light" icon={<IconDownload size={16} />}>
          <Group justify="space-between" wrap="wrap" gap="sm">
            <Text size="sm">
              {instancesWithUpdate.length} server{instancesWithUpdate.length === 1 ? ' has' : 's have'} a game update
              available
            </Text>
            <Anchor component={Link} to={`/instances/${firstUpdateInstance.id}/overview`} size="sm">
              View
            </Anchor>
          </Group>
        </Alert>
      )}
    </Stack>
  )
}
