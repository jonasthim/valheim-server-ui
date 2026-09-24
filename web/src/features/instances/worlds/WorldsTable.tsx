import { ActionIcon, Badge, Button, Group, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconDownload, IconRefreshAlert, IconTrash } from '@tabler/icons-react'
import { openConfirmWorldAction } from './openConfirmWorldAction'
import { api } from '../../../api/client'
import type { World } from '../../../api/types'
import { fmtAgo, fmtBytes } from '../../../lib/format'
import { DataTable, EmptyState } from '../../../ui'
import type { DataTableColumn } from '../../../ui'

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

  const columns: DataTableColumn<World>[] = [
    {
      key: 'name',
      header: 'Name',
      render: (w) => (
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
              {/* Focusable with the explanation as its name, so the
                  tooltip's text is reachable without a pointer. */}
              <Badge
                color="yellow"
                variant="light"
                style={{ cursor: 'help' }}
                tabIndex={0}
                aria-label="Not saved yet: Valheim writes the world data at the first save; until then there is nothing to back up or download."
              >
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
      ),
    },
    { key: 'size', header: 'Size', render: (w) => fmtBytes(w.size_bytes) },
    { key: 'modified', header: 'Modified', render: (w) => fmtAgo(w.modified_at) },
  ]

  return (
    <DataTable
      columns={columns}
      rows={worlds}
      rowKey={(w) => w.name}
      loading={isLoading}
      minWidth={720}
      empty={
        <EmptyState
          compact
          title="No worlds found in the save directory yet."
          description="Upload a world or start the server to create one."
        />
      }
      actions={
        canManage
          ? (w) => (
              <Group gap="xs" wrap="nowrap" justify="flex-end">
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
                {w.active ? (
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
                ) : (
                  <>
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
                  </>
                )}
              </Group>
            )
          : undefined
      }
    />
  )
}
