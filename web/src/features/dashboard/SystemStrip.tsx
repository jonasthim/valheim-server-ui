// System-health alerts shown above the instance grid: SteamCMD readiness,
// the manager's own update banner (WP-32), and a rollup of instances with a
// game update available. The version/disk/manager KPIs themselves live in
// the DashboardPage stat row.
import { Alert, Anchor, Group, Skeleton, Stack, Text } from '@mantine/core'
import { IconAlertTriangle, IconDownload } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import type { Instance } from '../../api/types'
import { AppUpdateBanner, useSystemInfo } from '../system'

export function SystemStrip({ instances }: { instances: Instance[] }) {
  const query = useSystemInfo()

  if (query.isLoading) return <Skeleton height={40} radius="lg" />
  if (!query.data) return null

  const info = query.data
  const instancesWithUpdate = instances.filter((i) => i.status.update_available)
  const firstUpdateInstance = instancesWithUpdate[0]

  return (
    <Stack gap="sm">
      {!info.steamcmd_installed && (
        <Alert
          color="red"
          variant="light"
          radius="lg"
          icon={<IconAlertTriangle size={16} />}
          title="SteamCMD is not installed"
        >
          Run the installer (see docs/RUNBOOK.md) before installing any instance.
        </Alert>
      )}

      <AppUpdateBanner appUpdate={info.app_update} />

      {firstUpdateInstance && (
        <Alert color="blue" variant="light" radius="lg" icon={<IconDownload size={16} />}>
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
