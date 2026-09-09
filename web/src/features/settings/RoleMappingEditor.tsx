import { useEffect, useState } from 'react'
import { ActionIcon, Button, Group, Select, Stack, Text, TextInput } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import type { Role } from '../../api/types'
import { ROLE_MAP_OPTIONS } from './options'

interface Row {
  id: string
  group: string
  role: Role
}

function toRows(value: Record<string, Role>): Row[] {
  return Object.entries(value).map(([group, role]) => ({ id: crypto.randomUUID(), group, role }))
}

function toRecord(rows: Row[]): Record<string, Role> {
  const record: Record<string, Role> = {}
  for (const row of rows) {
    const group = row.group.trim()
    if (group) record[group] = row.role
  }
  return record
}

/** Group name -> role editor. `resetToken` re-derives rows from `value` (used when settings (re)load). */
export function RoleMappingEditor({
  value,
  onChange,
  resetToken,
}: {
  value: Record<string, Role>
  onChange: (value: Record<string, Role>) => void
  resetToken: number
}) {
  const [rows, setRows] = useState<Row[]>(() => toRows(value))

  useEffect(() => {
    setRows(toRows(value))
    // Only re-derive when settings data (re)loads, not on every keystroke.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [resetToken])

  function update(next: Row[]) {
    setRows(next)
    onChange(toRecord(next))
  }

  return (
    <div>
      {rows.length === 0 && (
        <Text size="sm" c="dimmed" mb="xs">
          No group mappings configured.
        </Text>
      )}
      <Stack gap="xs" mb="xs">
        {rows.map((row, i) => (
          <Group key={row.id} gap="xs" wrap="nowrap">
            <TextInput
              placeholder="Group name"
              value={row.group}
              onChange={(e) => update(rows.map((r, j) => (j === i ? { ...r, group: e.currentTarget.value } : r)))}
              style={{ flex: 1 }}
              aria-label="Group name"
            />
            <Select
              data={ROLE_MAP_OPTIONS}
              value={row.role}
              onChange={(v) => v && update(rows.map((r, j) => (j === i ? { ...r, role: v as Role } : r)))}
              allowDeselect={false}
              w={140}
              aria-label="Role"
            />
            <ActionIcon
              variant="subtle"
              color="red"
              onClick={() => update(rows.filter((_, j) => j !== i))}
              aria-label="Remove mapping"
            >
              <IconTrash size={16} />
            </ActionIcon>
          </Group>
        ))}
      </Stack>
      <Button
        variant="subtle"
        size="xs"
        leftSection={<IconPlus size={14} />}
        onClick={() => update([...rows, { id: crypto.randomUUID(), group: '', role: 'viewer' }])}
      >
        Add mapping
      </Button>
    </div>
  )
}
