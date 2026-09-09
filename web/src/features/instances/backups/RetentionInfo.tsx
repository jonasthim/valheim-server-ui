import { Text } from '@mantine/core'
import { Link } from 'react-router-dom'
import type { InstanceConfig } from '../../../api/types'

export function RetentionInfo({ id, config }: { id: string; config: InstanceConfig | undefined }) {
  if (!config) return null
  return (
    <Text size="xs" c="dimmed">
      Scheduled, pre-update and pre-restore backups keep the newest {config.backup_keep_last ?? 10} and anything
      newer than {config.backup_keep_days ?? 30} days; manual and uploaded backups are never auto-deleted. Change
      this in <Link to={`/instances/${id}/config`}>Config</Link>.
    </Text>
  )
}
