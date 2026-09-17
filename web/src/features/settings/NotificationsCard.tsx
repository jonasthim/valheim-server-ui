import { useEffect, useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import {
  ActionIcon,
  Button,
  Divider,
  Group,
  MultiSelect,
  NumberInput,
  PasswordInput,
  Select,
  Stack,
  Switch,
  Text,
  TextInput,
} from '@mantine/core'
import { IconPlus, IconSend, IconTrash } from '@tabler/icons-react'
import { api } from '../../api/client'
import type { AlertKind, NotifyChannel, NotifyChannelType, NotifySettings } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'
import { SectionCard } from '../../ui'
import { useInstances } from '../instances'

const CHANNEL_TYPE_OPTIONS: { value: NotifyChannelType; label: string }[] = [
  { value: 'discord', label: 'Discord' },
  { value: 'slack', label: 'Slack' },
  { value: 'ntfy', label: 'ntfy' },
  { value: 'telegram', label: 'Telegram' },
  { value: 'webhook', label: 'Webhook' },
  { value: 'email', label: 'Email' },
]

const ALERT_KIND_OPTIONS: { value: AlertKind; label: string }[] = [
  { value: 'crashed', label: 'Instance crashed' },
  { value: 'down', label: 'Instance down' },
  { value: 'job_failed', label: 'Job failed' },
  { value: 'game_update', label: 'Game update available' },
  { value: 'app_update', label: 'Manager update available' },
  { value: 'disk_low', label: 'Disk space low' },
  { value: 'player_join', label: 'Player joined' },
  { value: 'player_leave', label: 'Player left' },
]

// discord/slack/telegram URLs carry the credential (webhook path / bot
// token), so GET /settings never returns them for these types; a blank
// submit keeps the stored value, same as secret.
const URL_HIDDEN_TYPES: NotifyChannelType[] = ['discord', 'slack', 'telegram']

interface ChannelRow extends NotifyChannel {
  key: string
}

function emptyRow(): ChannelRow {
  return { key: crypto.randomUUID(), id: '', type: 'discord', name: '', enabled: true, url: '', secret: '', events: [], instances: [] }
}

function toRows(channels: NotifyChannel[]): ChannelRow[] {
  return channels.map((c) => ({ ...c, key: crypto.randomUUID() }))
}

function toChannels(rows: ChannelRow[]): NotifyChannel[] {
  return rows.map(({ key: _key, ...c }) => c)
}

function TestButton({ channelId }: { channelId: string }) {
  const test = useMutation({
    mutationFn: () => api.post<{ ok: boolean }>(`/settings/notifications/${channelId}/test`),
    onSuccess: () => notifySuccess('Test notification sent'),
    onError: (err) => notifyError(err, 'Test notification failed'),
  })
  return (
    <Button
      type="button"
      variant="default"
      size="xs"
      leftSection={<IconSend size={14} />}
      loading={test.isPending}
      onClick={() => test.mutate()}
    >
      Send test
    </Button>
  )
}

function ChannelRowEditor({
  row,
  instanceOptions,
  onChange,
  onRemove,
}: {
  row: ChannelRow
  instanceOptions: { value: string; label: string }[]
  onChange: (next: ChannelRow) => void
  onRemove: () => void
}) {
  const urlHidden = URL_HIDDEN_TYPES.includes(row.type)
  const hasStoredValue = row.id !== ''

  return (
    <Stack gap="xs" p="sm" bd="1px solid var(--mantine-color-default-border)" style={{ borderRadius: 8 }}>
      <Group gap="xs" wrap="wrap" align="flex-end">
        <Select
          label="Type"
          data={CHANNEL_TYPE_OPTIONS}
          value={row.type}
          onChange={(v) => v && onChange({ ...row, type: v as NotifyChannelType })}
          allowDeselect={false}
          w={140}
        />
        <TextInput
          label="Name"
          value={row.name}
          onChange={(e) => onChange({ ...row, name: e.currentTarget.value })}
          style={{ flex: 1, minWidth: 160 }}
        />
        <Switch
          label="Enabled"
          checked={row.enabled}
          onChange={(e) => onChange({ ...row, enabled: e.currentTarget.checked })}
        />
        <ActionIcon variant="subtle" color="red" onClick={onRemove} aria-label="Remove channel">
          <IconTrash size={16} />
        </ActionIcon>
      </Group>

      <TextInput
        label="URL"
        placeholder={urlHidden && hasStoredValue ? 'unchanged (hidden)' : undefined}
        description={urlHidden ? 'Carries the webhook/bot credential; never shown again after saving. Leave blank to keep it.' : undefined}
        value={row.url ?? ''}
        onChange={(e) => onChange({ ...row, url: e.currentTarget.value })}
      />
      <PasswordInput
        label="Secret"
        placeholder={hasStoredValue ? 'unchanged' : undefined}
        description="Leave blank to keep the current value"
        value={row.secret ?? ''}
        onChange={(e) => onChange({ ...row, secret: e.currentTarget.value })}
      />
      <MultiSelect
        label="Events"
        placeholder="Alert kinds this channel receives"
        data={ALERT_KIND_OPTIONS}
        value={row.events}
        onChange={(v) => onChange({ ...row, events: v as AlertKind[] })}
      />
      <MultiSelect
        label="Instances"
        placeholder="All instances"
        description="Leave empty to apply to every instance"
        data={instanceOptions}
        value={row.instances}
        onChange={(v) => onChange({ ...row, instances: v })}
      />

      <Group justify="flex-end">
        {hasStoredValue ? (
          <TestButton channelId={row.id} />
        ) : (
          <Text size="xs" c="dimmed">
            Save settings before sending a test
          </Text>
        )}
      </Group>
    </Stack>
  )
}

/** Editor for `form.values.notifications`, mirroring RoleMappingEditor's local-rows-plus-resetToken pattern. */
export function NotificationsCard({
  value,
  onChange,
  resetToken,
}: {
  value: NotifySettings
  onChange: (next: NotifySettings) => void
  resetToken: number
}) {
  const [rows, setRows] = useState<ChannelRow[]>(() => toRows(value.channels))
  const instancesQ = useInstances()
  const instanceOptions = (instancesQ.data ?? []).map((i) => ({ value: i.id, label: i.name }))

  useEffect(() => {
    setRows(toRows(value.channels))
    // Only re-derive rows when settings data (re)loads, not on every keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetToken])

  function update(next: ChannelRow[]) {
    setRows(next)
    onChange({ ...value, channels: toChannels(next) })
  }

  return (
    <SectionCard title="Notifications">
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          Push alerts to Discord, Slack, ntfy, Telegram, a webhook or email when an instance crashes or goes down, a
          job fails, an update is available, disk space runs low, or players join/leave.
        </Text>

        {rows.length === 0 && (
          <Text size="sm" c="dimmed">
            No notification channels configured.
          </Text>
        )}
        <Stack gap="sm">
          {rows.map((row, i) => (
            <ChannelRowEditor
              key={row.key}
              row={row}
              instanceOptions={instanceOptions}
              onChange={(next) => update(rows.map((r, j) => (j === i ? next : r)))}
              onRemove={() => update(rows.filter((_, j) => j !== i))}
            />
          ))}
        </Stack>
        <Group>
          <Button variant="subtle" size="xs" leftSection={<IconPlus size={14} />} onClick={() => update([...rows, emptyRow()])}>
            Add channel
          </Button>
        </Group>

        <Divider />
        <NumberInput
          label="Disk low threshold (%)"
          description="Alert when free space drops below this percentage; 0 uses the default (10%)"
          min={0}
          max={100}
          value={value.disk_low_percent}
          onChange={(v) => onChange({ ...value, disk_low_percent: typeof v === 'number' ? v : 0 })}
        />
      </Stack>
    </SectionCard>
  )
}
