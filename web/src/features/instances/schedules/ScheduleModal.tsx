import { useState } from 'react'
import { Button, Divider, Group, Modal, NumberInput, Select, Stack, Switch, Text, TextInput, Textarea } from '@mantine/core'
import { useForm } from '@mantine/form'
import { ApiError } from '../../../api/client'
import type { AgentCommandRequest, Schedule, ScheduleInput, ScheduleKind } from '../../../api/types'
import { CRON_PRESETS, CUSTOM_CRON_PRESET, cronDescribe, presetForExpr, validateCronExpr } from './cron'
import { SCHEDULE_KIND_HELP, SCHEDULE_KIND_OPTIONS } from './constants'
import { useCreateSchedule, useUpdateSchedule } from './useSchedules'

// A restart schedule with no lead_seconds override (0) uses the scheduler's
// own default; mirror that default in the form so the field reads sensibly.
const DEFAULT_RESTART_LEAD_SECONDS = 120

interface FormValues {
  kind: ScheduleKind
  preset: string
  cron: string
  enabled: boolean
  only_when_empty: boolean
  note: string
  message: string
  command: string
  target: string
  lead_seconds: number
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
      message: (schedule?.kind === 'command' ? schedule.command?.message : schedule?.message) ?? '',
      command: schedule?.command?.command ?? '',
      target: schedule?.command?.target ?? '',
      lead_seconds: schedule?.lead_seconds || DEFAULT_RESTART_LEAD_SECONDS,
    },
    validate: {
      note: (v) => (v.length <= 100 ? null : 'Max 100 characters'),
      message: (v, values) =>
        values.kind === 'announce' && !v.trim() ? 'Required' : v.length <= 500 ? null : 'Max 500 characters',
      command: (v, values) => (values.kind === 'command' && !v.trim() ? 'Required' : null),
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
      // lead_seconds only means something for "restart"; send 0 (unset) for
      // every other kind rather than a stale value from switching kinds.
      lead_seconds: values.kind === 'restart' ? values.lead_seconds : 0,
    }
    if (values.kind === 'announce') {
      input.message = values.message.trim()
    } else if (values.kind === 'command') {
      input.command = {
        command: values.command.trim() as AgentCommandRequest['command'],
        target: values.target.trim() || undefined,
        message: values.message.trim() || undefined,
      }
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

            {form.values.kind === 'announce' && (
              <Textarea
                label="Message"
                placeholder="Server restarting in 5 minutes"
                maxLength={500}
                autosize
                minRows={2}
                {...form.getInputProps('message')}
              />
            )}

            {form.values.kind === 'command' && (
              <Stack gap="sm">
                <TextInput
                  label="Command"
                  placeholder="broadcast, say, save, kick, ban, unban, ..."
                  {...form.getInputProps('command')}
                />
                <TextInput
                  label="Target"
                  placeholder="player name or id (kick, ban, unban)"
                  {...form.getInputProps('target')}
                />
                <TextInput
                  label="Message"
                  placeholder="text (broadcast, say)"
                  {...form.getInputProps('message')}
                />
              </Stack>
            )}

            {form.values.kind === 'restart' && (
              <NumberInput
                label="Warn players (seconds)"
                description="How long before the restart to start warning connected players."
                min={0}
                max={3600}
                {...form.getInputProps('lead_seconds')}
              />
            )}

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
