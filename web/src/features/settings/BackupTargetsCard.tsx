import { useEffect, useState } from 'react'
import { ActionIcon, Button, Group, NumberInput, Select, Stack, Text, TextInput } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import type { BackupTarget, BackupTargetType } from '../../api/types'
import { newKey } from '../../lib/keys'
import { SectionCard } from '../../ui'

const TARGET_TYPE_OPTIONS: { value: BackupTargetType; label: string }[] = [
  { value: 'local', label: 'Local path' },
  { value: 'rclone', label: 'rclone remote' },
]

interface TargetRow extends BackupTarget {
  key: string
}

function emptyRow(): TargetRow {
  return { key: newKey(), id: '', type: 'local', name: '', path: '', remote: '', keep_last: 0 }
}

function toRows(targets: BackupTarget[] | undefined): TargetRow[] {
  return (targets ?? []).map((t) => ({ ...t, key: newKey() }))
}

function toTargets(rows: TargetRow[]): BackupTarget[] {
  return rows.map(({ key: _key, ...t }) => t)
}

function TargetRowEditor({
  row,
  onChange,
  onRemove,
}: {
  row: TargetRow
  onChange: (next: TargetRow) => void
  onRemove: () => void
}) {
  return (
    <Stack gap="xs" p="sm" bd="1px solid var(--mantine-color-default-border)" style={{ borderRadius: 8 }}>
      <Group gap="xs" wrap="wrap" align="flex-end">
        <Select
          label="Type"
          data={TARGET_TYPE_OPTIONS}
          value={row.type}
          onChange={(v) => v && onChange({ ...row, type: v as BackupTargetType })}
          allowDeselect={false}
          w={160}
        />
        <TextInput
          label="Name"
          value={row.name}
          onChange={(e) => onChange({ ...row, name: e.currentTarget.value })}
          style={{ flex: 1, minWidth: 160 }}
        />
        <NumberInput
          label="Keep last"
          description="0 = no pruning"
          min={0}
          max={1000}
          value={row.keep_last}
          onChange={(v) => onChange({ ...row, keep_last: typeof v === 'number' ? v : 0 })}
          w={120}
        />
        <ActionIcon variant="subtle" color="red" onClick={onRemove} aria-label="Remove target">
          <IconTrash size={16} />
        </ActionIcon>
      </Group>

      {row.type === 'local' ? (
        <TextInput
          label="Local directory"
          description="Absolute path on the manager host, e.g. a mounted network share"
          placeholder="/mnt/backups/valheim"
          value={row.path ?? ''}
          onChange={(e) => onChange({ ...row, path: e.currentTarget.value })}
        />
      ) : (
        <TextInput
          label="rclone remote"
          description='rclone remote:path, e.g. "b2:my-bucket/valheim"'
          placeholder="remote:bucket/path"
          value={row.remote ?? ''}
          onChange={(e) => onChange({ ...row, remote: e.currentTarget.value })}
        />
      )}
    </Stack>
  )
}

/**
 * Editor for `form.values.backups.targets`, mirroring NotificationsCard's
 * local-rows-plus-resetToken pattern. Each instance then picks one of these
 * targets (by id/name only) and which backup kinds to copy, in
 * InstanceConfigForm's "Saves & backups" section.
 */
export function BackupTargetsCard({
  value,
  onChange,
  resetToken,
}: {
  value: BackupTarget[]
  onChange: (next: BackupTarget[]) => void
  resetToken: number
}) {
  const [rows, setRows] = useState<TargetRow[]>(() => toRows(value))

  useEffect(() => {
    setRows(toRows(value))
    // Only re-derive rows when settings data (re)loads, not on every keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetToken])

  function update(next: TargetRow[]) {
    setRows(next)
    onChange(toTargets(next))
  }

  return (
    <SectionCard title="Backup targets">
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          Off-box destinations instances can copy their backups to after each run: a local path (e.g. a mounted
          network share) or any remote rclone supports. Pruning a remote target is not managed here; the local
          option prunes to "keep last" itself.
        </Text>

        {rows.length === 0 && (
          <Text size="sm" c="dimmed">
            No backup targets configured.
          </Text>
        )}
        <Stack gap="sm">
          {rows.map((row, i) => (
            <TargetRowEditor
              key={row.key}
              row={row}
              onChange={(next) => update(rows.map((r, j) => (j === i ? next : r)))}
              onRemove={() => update(rows.filter((_, j) => j !== i))}
            />
          ))}
        </Stack>
        <Group>
          <Button variant="subtle" size="xs" leftSection={<IconPlus size={14} />} onClick={() => update([...rows, emptyRow()])}>
            Add target
          </Button>
        </Group>
      </Stack>
    </SectionCard>
  )
}
