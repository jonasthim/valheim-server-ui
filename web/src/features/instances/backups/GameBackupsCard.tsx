import { ActionIcon, Badge, Checkbox, Stack, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconDatabase, IconRestore } from '@tabler/icons-react'
import type { GameBackup, InstanceState } from '../../../api/types'
import { fmtAgo, fmtBytes, fmtTime } from '../../../lib/format'
import { DataTable, EmptyState, LoadError, SectionCard } from '../../../ui'
import type { DataTableColumn } from '../../../ui'
import { useGameBackups, useRestoreGameBackup } from './useGameBackups'

export function GameBackupsCard({
  id,
  canManage,
  instanceState,
}: {
  id: string
  canManage: boolean
  instanceState: InstanceState | undefined
}) {
  const q = useGameBackups(id)
  const restore = useRestoreGameBackup(id)
  const running = instanceState === 'running'

  function confirmRestore(c: GameBackup) {
    let stopIfRunning = running
    modals.openConfirmModal({
      title: 'Restore game backup',
      children: (
        <Stack gap="sm">
          <Text size="sm">
            Restore <strong>{c.name}</strong> over world <strong>{c.world}</strong>? A pre-restore backup of the
            current save is taken first. The instance must be stopped for the restore to run.
          </Text>
          {running && (
            <Checkbox
              label="Stop, restore and start again"
              defaultChecked={stopIfRunning}
              onChange={(e) => {
                stopIfRunning = e.currentTarget.checked
              }}
            />
          )}
        </Stack>
      ),
      labels: { confirm: 'Restore', cancel: 'Cancel' },
      confirmProps: { color: 'straw' },
      onConfirm: () => restore.mutate({ name: c.name, stopIfRunning }),
    })
  }

  const columns: DataTableColumn<GameBackup>[] = [
    {
      key: 'created',
      header: 'Created',
      sortable: true,
      sortValue: (c) => c.created_at,
      render: (c) => (
        <>
          <Text size="sm">{fmtTime(c.created_at)}</Text>
          <Text size="xs" c="dimmed">
            {fmtAgo(c.created_at)}
          </Text>
        </>
      ),
    },
    { key: 'world', header: 'World', render: (c) => c.world },
    {
      key: 'kind',
      header: 'Kind',
      render: (c) => (
        <Badge variant="light" color="gray">
          {c.kind}
        </Badge>
      ),
    },
    { key: 'size', header: 'Size', render: (c) => fmtBytes(c.size_bytes) },
    {
      key: 'copy',
      header: 'Copy',
      render: (c) => (
        <Text size="xs" ff="monospace" truncate maw={320} title={c.name}>
          {c.name}
        </Text>
      ),
    },
  ]

  return (
    <SectionCard
      title="Game backups"
      description="Valheim's own rolling copies of the world, written by the game server. Restoring one takes a manager backup of the current save first."
      flush
    >
      <DataTable
        aria-label="Game backups"
        columns={columns}
        rows={q.data ?? []}
        rowKey={(c) => c.name}
        loading={q.isLoading}
        error={
          q.isError ? <LoadError error={q.error} title="Could not load game backups" onRetry={() => q.refetch()} /> : undefined
        }
        defaultSort={{ key: 'created', dir: 'desc' }}
        minWidth={760}
        empty={
          <EmptyState
            compact
            icon={<IconDatabase size={22} />}
            title="No game backups yet"
            description="Valheim writes them while the server runs; Config → Game backups to keep sets how many."
          />
        }
        actions={
          canManage
            ? (c) => (
                <Tooltip label={c.restorable ? 'Restore' : 'This copy has no committed save'}>
                  {/* data-disabled span keeps the tooltip reachable on a disabled button */}
                  <span>
                    <ActionIcon
                      variant="subtle"
                      color="straw"
                      aria-label={`Restore ${c.name}`}
                      disabled={!c.restorable}
                      onClick={() => confirmRestore(c)}
                    >
                      <IconRestore size={16} />
                    </ActionIcon>
                  </span>
                </Tooltip>
              )
            : undefined
        }
      />
    </SectionCard>
  )
}
