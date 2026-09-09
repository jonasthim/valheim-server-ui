import { ActionIcon, Badge, Button, Group, Skeleton, Table, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconDownload, IconTrash } from '@tabler/icons-react'
import { api } from '../../../api/client'
import type { World } from '../../../api/types'
import { fmtAgo, fmtBytes } from '../../../lib/format'

export function WorldsTable({
  id,
  worlds,
  isLoading,
  canManage,
  onMakeActive,
  onDelete,
  makeActivePending,
  deletePending,
}: {
  id: string
  worlds: World[]
  isLoading: boolean
  canManage: boolean
  onMakeActive: (name: string) => void
  onDelete: (name: string) => void
  makeActivePending: boolean
  deletePending: boolean
}) {
  function confirmMakeActive(name: string) {
    modals.openConfirmModal({
      title: 'Make active world',
      children: (
        <Text size="sm">
          Set <strong>{name}</strong> as the active world? The instance must be restarted for this to take effect;
          players already connected keep playing on the current world until then.
        </Text>
      ),
      labels: { confirm: 'Make active', cancel: 'Cancel' },
      onConfirm: () => onMakeActive(name),
    })
  }

  function confirmDelete(name: string) {
    modals.openConfirmModal({
      title: 'Delete world',
      children: (
        <Text size="sm">
          Delete <strong>{name}</strong> and its save files? This cannot be undone; consider a backup first.
        </Text>
      ),
      labels: { confirm: 'Delete', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => onDelete(name),
    })
  }

  if (isLoading) return <Skeleton height={120} />

  if (worlds.length === 0) {
    return (
      <Text c="dimmed" size="sm">
        No worlds found in the save directory yet.
      </Text>
    )
  }

  return (
    <Table.ScrollContainer minWidth={720}>
      <Table verticalSpacing="xs" highlightOnHover>
        <Table.Thead>
          <Table.Tr>
            <Table.Th>Name</Table.Th>
            <Table.Th>Size</Table.Th>
            <Table.Th>Modified</Table.Th>
            <Table.Th />
            <Table.Th />
          </Table.Tr>
        </Table.Thead>
        <Table.Tbody>
          {worlds.map((w) => (
            <Table.Tr key={w.name}>
              <Table.Td>
                <Group gap="xs">
                  <Text fw={500}>{w.name}</Text>
                  {w.active && (
                    <Badge color="green" variant="light">
                      Active
                    </Badge>
                  )}
                  {w.has_db === false && (
                    <Badge color="red" variant="light">
                      Missing .db
                    </Badge>
                  )}
                  {w.has_fwl === false && (
                    <Badge color="red" variant="light">
                      Missing .fwl
                    </Badge>
                  )}
                </Group>
              </Table.Td>
              <Table.Td>{fmtBytes(w.size_bytes)}</Table.Td>
              <Table.Td>{fmtAgo(w.modified_at)}</Table.Td>
              <Table.Td>
                {canManage && (
                  <Tooltip label="Download as zip">
                    <ActionIcon
                      component="a"
                      href={api.url(`/instances/${id}/worlds/${encodeURIComponent(w.name)}/download`)}
                      variant="subtle"
                      aria-label={`Download ${w.name}`}
                    >
                      <IconDownload size={16} />
                    </ActionIcon>
                  </Tooltip>
                )}
              </Table.Td>
              <Table.Td>
                {canManage && !w.active && (
                  <Group gap="xs" wrap="nowrap" justify="flex-end">
                    <Button size="xs" variant="light" loading={makeActivePending} onClick={() => confirmMakeActive(w.name)}>
                      Make active
                    </Button>
                    <Tooltip label="Delete">
                      <ActionIcon
                        variant="subtle"
                        color="red"
                        aria-label={`Delete ${w.name}`}
                        loading={deletePending}
                        onClick={() => confirmDelete(w.name)}
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
