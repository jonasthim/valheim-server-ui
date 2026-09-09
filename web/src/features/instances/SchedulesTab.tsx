import { useState } from 'react'
import { Button, Group, Stack, Title } from '@mantine/core'
import { IconPlus } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { ScheduleModal } from './schedules/ScheduleModal'
import { SchedulesTable } from './schedules/SchedulesTable'
import { useDeleteSchedule, useRunSchedule, useSchedules, useUpdateSchedule } from './schedules/useSchedules'
import type { Schedule } from '../../api/types'

// Owned by WP-12 (docs/WORKPLAN.md). Props: the instance id.
export function SchedulesTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const canManage = hasRole('operator')
  const schedulesQ = useSchedules(id)
  const update = useUpdateSchedule(id)
  const del = useDeleteSchedule(id)
  const run = useRunSchedule(id)

  const [modalOpened, setModalOpened] = useState(false)
  const [editing, setEditing] = useState<Schedule | undefined>(undefined)

  function openCreate() {
    setEditing(undefined)
    setModalOpened(true)
  }

  function openEdit(schedule: Schedule) {
    setEditing(schedule)
    setModalOpened(true)
  }

  return (
    <Stack gap="md">
      <Group justify="space-between">
        <Title order={4}>Schedules</Title>
        {canManage && (
          <Button leftSection={<IconPlus size={16} />} onClick={openCreate}>
            New schedule
          </Button>
        )}
      </Group>

      <SchedulesTable
        schedules={schedulesQ.data ?? []}
        isLoading={schedulesQ.isLoading}
        canManage={canManage}
        runPendingId={run.isPending ? (run.variables ?? null) : null}
        onToggleEnabled={(schedule, enabled) =>
          update.mutate({
            scheduleId: schedule.id,
            input: {
              kind: schedule.kind,
              cron: schedule.cron,
              enabled,
              only_when_empty: schedule.only_when_empty,
              note: schedule.note,
            },
          })
        }
        onRun={(scheduleId) => run.mutate(scheduleId)}
        onEdit={openEdit}
        onDelete={(scheduleId) => del.mutate(scheduleId)}
      />

      {canManage && (
        <ScheduleModal id={id} opened={modalOpened} onClose={() => setModalOpened(false)} schedule={editing} />
      )}
    </Stack>
  )
}
