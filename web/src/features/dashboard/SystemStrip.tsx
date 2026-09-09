// Compact system info strip shown above the instance grid.
import { Alert, Group, Paper, Skeleton, Text } from '@mantine/core'
import { useQuery } from '@tanstack/react-query'
import { IconAlertTriangle } from '@tabler/icons-react'
import { api } from '../../api/client'
import type { SystemInfo } from '../../api/types'
import { fmtBytes } from '../../lib/format'

export function SystemStrip() {
  const query = useQuery({
    queryKey: ['system'],
    queryFn: () => api.get<SystemInfo>('/system'),
    refetchInterval: 30_000,
  })

  if (query.isLoading) return <Skeleton height={48} />
  if (!query.data) return null

  const info = query.data

  return (
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
        {!info.steamcmd_installed && (
          <Alert color="red" icon={<IconAlertTriangle size={16} />} py={4} px="sm">
            SteamCMD is not installed. Run the installer (see docs/RUNBOOK.md) before installing any instance.
          </Alert>
        )}
      </Group>
    </Paper>
  )
}
