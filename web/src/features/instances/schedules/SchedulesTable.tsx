import type { ReactNode } from 'react'
import { ActionIcon, Anchor, Badge, Group, Switch, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconCalendarTime, IconPencil, IconPlayerPlay, IconTrash } from '@tabler/icons-react'
import type { Schedule } from '../../../api/types'
import { fmtAgo, fmtTime } from '../../../lib/format'
import { Dash, DataTable, EmptyState, StatusPill } from '../../../ui'
import type { DataTableColumn } from '../../../ui'
import { useJobDrawer } from '../../jobs'
import { cronDescribe } from './cron'
import { LAST_RESULT_COLORS, SCHEDULE_KIND_LABELS } from './constants'

// scheduleSummary is a short, kind-aware description shown where the note
// otherwise would be: an announce schedule's message, or a command
// schedule's "command target"; every other kind (and a command/announce
// schedule with nothing to summarize) falls back to the note.
function scheduleSummary(s: Schedule): ReactNode {
  if (s.kind === 'announce' && s.message) return s.message
  if (s.kind === 'command' && s.command) return [s.command.command, s.command.target].filter(Boolean).join(' ')
  return s.note || <Dash />
}

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

  const columns: DataTableColumn<Schedule>[] = [
    {
      key: 'kind',
      header: 'Kind',
      nowrap: true,
      render: (s) => (
        <Badge variant="light" style={{ overflow: 'visible' }}>
          {SCHEDULE_KIND_LABELS[s.kind]}
        </Badge>
      ),
    },
    {
      key: 'schedule',
      header: 'Schedule',
      render: (s) => (
        <>
          <Text size="sm">{cronDescribe(s.cron)}</Text>
          <Text size="xs" c="dimmed" ff="monospace">
            {s.cron}
          </Text>
        </>
      ),
    },
    {
      key: 'enabled',
      header: 'Enabled',
      render: (s) => (
        <Switch
          checked={s.enabled}
          disabled={!canManage}
          onChange={(e) => onToggleEnabled(s, e.currentTarget.checked)}
          aria-label={`Enable schedule ${s.id}`}
        />
      ),
    },
    { key: 'only_when_empty', header: 'Only when empty', render: (s) => (s.only_when_empty ? 'Yes' : 'No') },
    { key: 'note', header: 'Note', render: (s) => scheduleSummary(s) },
    { key: 'next_run', header: 'Next run', render: (s) => (s.next_run_at ? fmtTime(s.next_run_at) : <Dash />) },
    {
      key: 'last_run',
      header: 'Last run',
      render: (s) =>
        s.last_run_at ? (
          <Group gap={6} wrap="nowrap">
            <Text size="sm">{fmtAgo(s.last_run_at)}</Text>
            {s.last_result && <StatusPill color={LAST_RESULT_COLORS[s.last_result]}>{s.last_result}</StatusPill>}
            {s.last_job_id && (
              <Anchor size="xs" component="button" type="button" onClick={() => openJob(s.last_job_id!)}>
                job
              </Anchor>
            )}
          </Group>
        ) : (
          <Dash />
        ),
    },
  ]

  return (
    <DataTable
      columns={columns}
      rows={schedules}
      rowKey={(s) => s.id}
      loading={isLoading}
      minWidth={960}
      empty={
        <EmptyState
          icon={<IconCalendarTime size={22} />}
          title="No schedules configured."
          description="Add a restart, backup or update schedule."
        />
      }
      actions={
        canManage
          ? (s) => (
              <Group gap={4} wrap="nowrap" justify="flex-end">
                <Tooltip label="Run now">
                  <ActionIcon variant="subtle" aria-label="Run now" loading={runPendingId === s.id} onClick={() => onRun(s.id)}>
                    <IconPlayerPlay size={16} />
                  </ActionIcon>
                </Tooltip>
                <Tooltip label="Edit">
                  <ActionIcon variant="subtle" aria-label="Edit schedule" onClick={() => onEdit(s)}>
                    <IconPencil size={16} />
                  </ActionIcon>
                </Tooltip>
                <Tooltip label="Delete">
                  <ActionIcon variant="subtle" color="red" aria-label="Delete schedule" onClick={() => confirmDelete(s)}>
                    <IconTrash size={16} />
                  </ActionIcon>
                </Tooltip>
              </Group>
            )
          : undefined
      }
    />
  )
}
