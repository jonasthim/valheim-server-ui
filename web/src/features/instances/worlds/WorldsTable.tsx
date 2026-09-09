import { ActionIcon, Badge, Button, Group, Skeleton, Table, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconDownload, IconRefreshAlert, IconTrash } from '@tabler/icons-react'
import { openConfirmWorldAction } from './openConfirmWorldAction'
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
  onRegenerate,
  instanceRunning,
  makeActivePending,
  deletePending,
  regeneratePending,
}: {
  id: string
  worlds: World[]
  isLoading: boolean
  canManage: boolean
  onMakeActive: (name: string) => void
  onDelete: (name: string) => void
  onRegenerate: (name: string, stopIfRunning: boolean) => void
  instanceRunning: boolean
  makeActivePending: boolean
  deletePending: boolean
  regeneratePending: boolean
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
    openConfirmWorldAction({ worldName: name, action: 'delete', instanceRunning, onConfirm: () => onDelete(name) })
  }

  function confirmRegenerate(name: string) {
    openConfirmWorldAction({
      worldName: name,
      action: 'regenerate',
      instanceRunning,
      onConfirm: ({ stopIfRunning }) => onRegenerate(name, stopIfRunning),
    })
  }

  if (isLoading) {
    return (
      <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
        <Skeleton height={120} />
      </div>
    )
  }

  if (worlds.length === 0) {
    return (
      <Text c="dimmed" size="sm" p="lg">
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
                  {w.has_db === false && w.has_fwl !== false && (
                    <Tooltip
                      multiline
                      w={320}
                      label="Valheim writes the world metadata immediately and the world data at the first save: every save interval (30 min by default) or when the server stops. Until then the world has no data to back up or download."
                    >
                      <Badge color="yellow" variant="light" style={{ cursor: 'help' }}>
                        Not saved yet
                      </Badge>
                    </Tooltip>
                  )}
                  {w.has_fwl === false && (
                    <Badge color="red" variant="light">
                      Missing metadata
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
                {canManage && w.active && (
                  <Group gap="xs" wrap="nowrap" justify="flex-end">
                    <Tooltip label="Back up, delete the save files and let Valheim create a new world with a new seed">
                      <Button
                        size="xs"
                        variant="outline"
                        color="red"
                        leftSection={<IconRefreshAlert size={14} />}
                        loading={regeneratePending}
                        onClick={() => confirmRegenerate(w.name)}
                      >
                        Regenerate
                      </Button>
                    </Tooltip>
                  </Group>
                )}
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
