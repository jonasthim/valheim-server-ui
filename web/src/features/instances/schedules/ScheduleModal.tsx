import { useState } from 'react'
import { Button, Divider, Group, Modal, Select, Stack, Switch, Text, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { ApiError } from '../../../api/client'
import type { Schedule, ScheduleInput, ScheduleKind } from '../../../api/types'
import { CRON_PRESETS, CUSTOM_CRON_PRESET, cronDescribe, presetForExpr, validateCronExpr } from './cron'
import { SCHEDULE_KIND_HELP, SCHEDULE_KIND_OPTIONS } from './constants'
import { useCreateSchedule, useUpdateSchedule } from './useSchedules'

interface FormValues {
  kind: ScheduleKind
  preset: string
  cron: string
  enabled: boolean
  only_when_empty: boolean
  note: string
}

export function ScheduleModal({
  id,
  opened,
  onClose,
  schedule,
}: {
  id: string
  opened: boolean
  onClose: () => void
  schedule?: Schedule
}) {
  const isEdit = !!schedule
  const create = useCreateSchedule(id)
  const update = useUpdateSchedule(id)
  const [cronError, setCronError] = useState<string | null>(null)

  const form = useForm<FormValues>({
    initialValues: {
      kind: schedule?.kind ?? 'backup',
      cron: schedule?.cron ?? '0 4 * * *',
      preset: presetForExpr(schedule?.cron ?? '0 4 * * *'),
      enabled: schedule?.enabled ?? true,
      only_when_empty: schedule?.only_when_empty ?? true,
      note: schedule?.note ?? '',
    },
    validate: {
      note: (v) => (v.length <= 100 ? null : 'Max 100 characters'),
    },
  })

  const pending = create.isPending || update.isPending

  function submit(values: FormValues) {
    const err = validateCronExpr(values.cron)
    if (err) {
      setCronError(err)
      return
    }
    setCronError(null)
    const input: ScheduleInput = {
      kind: values.kind,
      cron: values.cron.trim(),
      enabled: values.enabled,
      only_when_empty: values.only_when_empty,
      note: values.note.trim() || undefined,
    }
    const onSuccess = () => {
      form.reset()
      onClose()
    }
    const onError = (e: unknown) => {
      if (e instanceof ApiError) {
        const fields = e.fieldErrors()
        if (fields.cron) setCronError(fields.cron)
        if (Object.keys(fields).length) {
          form.setErrors(fields)
          return
        }
      }
    }
    if (isEdit) {
      update.mutate({ scheduleId: schedule.id, input }, { onSuccess, onError })
    } else {
      create.mutate(input, { onSuccess, onError })
    }
  }

  return (
    <Modal opened={opened} onClose={onClose} title={isEdit ? 'Edit schedule' : 'New schedule'} radius="lg" centered>
      <form onSubmit={form.onSubmit(submit)}>
        <Stack gap="lg">
          <Stack gap="sm">
            <Select
              label="Kind"
              data={SCHEDULE_KIND_OPTIONS}
              allowDeselect={false}
              description={SCHEDULE_KIND_HELP[form.values.kind]}
              {...form.getInputProps('kind')}
            />

            <Select
              label="Schedule"
              data={CRON_PRESETS}
              allowDeselect={false}
              value={form.values.preset}
              onChange={(value) => {
                if (!value) return
                form.setFieldValue('preset', value)
                if (value !== CUSTOM_CRON_PRESET) {
                  form.setFieldValue('cron', value)
                  setCronError(null)
                }
              }}
            />

            <TextInput
              label="Cron expression"
              description={cronDescribe(form.values.cron)}
              error={cronError}
              value={form.values.cron}
              onChange={(e) => {
                const next = e.currentTarget.value
                form.setFieldValue('cron', next)
                form.setFieldValue('preset', presetForExpr(next))
                setCronError(null)
              }}
            />
          </Stack>

          <Divider label="Behavior" labelPosition="left" />

          <Stack gap="sm">
            <Switch
              label="Only run when the server is empty"
              checked={form.values.only_when_empty}
              onChange={(e) => form.setFieldValue('only_when_empty', e.currentTarget.checked)}
            />
            {form.values.kind === 'restart' && !form.values.only_when_empty && (
              <Text size="xs" c="straw">
                Valheim has no way to warn connected players before a restart.
              </Text>
            )}

            <Switch
              label="Enabled"
              checked={form.values.enabled}
              onChange={(e) => form.setFieldValue('enabled', e.currentTarget.checked)}
            />

            <TextInput label="Note (optional)" maxLength={100} {...form.getInputProps('note')} />
          </Stack>

          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={pending}>
              {isEdit ? 'Save' : 'Create'}
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  )
}
