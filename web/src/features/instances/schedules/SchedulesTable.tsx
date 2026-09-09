import { ActionIcon, Anchor, Badge, Group, Skeleton, Switch, Table, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconCalendarTime, IconPencil, IconPlayerPlay, IconTrash } from '@tabler/icons-react'
import type { Schedule } from '../../../api/types'
import { fmtAgo, fmtTime } from '../../../lib/format'
import { EmptyState, StatusPill } from '../../../ui'
import { useJobDrawer } from '../../jobs'
import { cronDescribe } from './cron'
import { LAST_RESULT_COLORS, SCHEDULE_KIND_LABELS } from './constants'

export function SchedulesTable({
  schedules,
  isLoading,
  canManage,
  onToggleEnabled,
  onRun,
  onEdit,
  onDelete,
  runPendingId,
}: {
  schedules: Schedule[]
  isLoading: boolean
  canManage: boolean
  onToggleEnabled: (schedule: Schedule, enabled: boolean) => void
  onRun: (scheduleId: number) => void
  onEdit: (schedule: Schedule) => void
  onDelete: (scheduleId: number) => void
  runPendingId: number | null
}) {
  const { openJob } = useJobDrawer()

  function confirmDelete(schedule: Schedule) {
    modals.openConfirmModal({
      title: 'Delete schedule',
      children: (
        <Text size="sm">
          Delete this {SCHEDULE_KIND_LABELS[schedule.kind].toLowerCase()} schedule ({schedule.cron})?
        </Text>
      ),
      labels: { confirm: 'Delete', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => onDelete(schedule.id),
    })
  }

  if (isLoading) {
    return (
      <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
        <Skeleton height={140} />
      </div>
    )
  }

  if (schedules.length === 0) {
    return (
      <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
        <EmptyState icon={<IconCalendarTime size={22} />} title="No schedules configured." />
      </div>
    )
  }

  return (
    <Table.ScrollContainer minWidth={960}>
      <Table verticalSpacing="xs">
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Kind</Table.Th>
            <Table.Th>Schedule</Table.Th>
            <Table.Th>Enabled</Table.Th>
            <Table.Th>Only when empty</Table.Th>
            <Table.Th>Note</Table.Th>
            <Table.Th>Next run</Table.Th>
            <Table.Th>Last run</Table.Th>
            <Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {schedules.map((s) => (
            <Table.Tr key={s.id}>
              <Table.Td style={{ whiteSpace: 'nowrap' }}>
                <Badge variant="light" style={{ overflow: 'visible' }}>
                  {SCHEDULE_KIND_LABELS[s.kind]}
                </Badge>
              </Table.Td>
              <Table.Td>
                <Text size="sm">{cronDescribe(s.cron)}</Text>
                <Text size="xs" c="dimmed" ff="monospace">
                  {s.cron}
                </Text>
              </Table.Td>
              <Table.Td>
                <Switch
                  checked={s.enabled}
                  disabled={!canManage}
                  onChange={(e) => onToggleEnabled(s, e.currentTarget.checked)}
                  aria-label={`Enable schedule ${s.id}`}
                />
              </Table.Td>
              <Table.Td>{s.only_when_empty ? 'Yes' : 'No'}</Table.Td>
              <Table.Td>{s.note || '-'}</Table.Td>
              <Table.Td>{s.next_run_at ? fmtTime(s.next_run_at) : '-'}</Table.Td>
              <Table.Td>
                {s.last_run_at ? (
                  <Group gap={6} wrap="nowrap">
                    <Text size="sm">{fmtAgo(s.last_run_at)}</Text>
                    {s.last_result && (
                      <StatusPill color={LAST_RESULT_COLORS[s.last_result]}>{s.last_result}</StatusPill>
                    )}
                    {s.last_job_id && (
                      <Anchor size="xs" component="button" type="button" onClick={() => openJob(s.last_job_id!)}>
                        job
                      </Anchor>
                    )}
                  </Group>
                ) : (
                  '-'
                )}
              </Table.Td>
              <Table.Td>
                {canManage && (
                  <Group gap={4} wrap="nowrap" justify="flex-end">
                    <Tooltip label="Run now">
                      <ActionIcon
                        variant="subtle"
                        aria-label="Run now"
                        loading={runPendingId === s.id}
                        onClick={() => onRun(s.id)}
                      >
                        <IconPlayerPlay size={16} />
                      </ActionIcon>
                    </Tooltip>
                    <Tooltip label="Edit">
                      <ActionIcon variant="subtle" aria-label="Edit schedule" onClick={() => onEdit(s)}>
                        <IconPencil size={16} />
                      </ActionIcon>
                    </Tooltip>
                    <Tooltip label="Delete">
                      <ActionIcon
                        variant="subtle"
                        color="red"
                        aria-label="Delete schedule"
                        onClick={() => confirmDelete(s)}
                      >
                        <IconTrash size={16} />
                      </ActionIcon>
                    </Tooltip>
                  </Group>
                )}
              </Table.Td>
            </Table.Tr>
          ))}
        </Table.Tbody>
      </Table>
    </Table.ScrollContainer>
  )
}
