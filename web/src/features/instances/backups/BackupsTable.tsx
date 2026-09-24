import type { ReactNode } from 'react'
import { ActionIcon, Badge, Checkbox, Group, Stack, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconDatabase, IconDownload, IconRefresh, IconRestore, IconTrash } from '@tabler/icons-react'
import { api } from '../../../api/client'
import type { Backup, InstanceState } from '../../../api/types'
import { fmtAgo, fmtBytes, fmtTime } from '../../../lib/format'
import { Dash, DataTable, EmptyState, SectionCard } from '../../../ui'
import type { DataTableColumn } from '../../../ui'
import { BACKUP_KIND_COLORS, BACKUP_KIND_LABELS } from './constants'
import { useRetryRemoteUpload } from './useBackups'

const REMOTE_STATUS_COLORS: Record<string, string> = {
  pending: 'straw',
  ok: 'moss',
  failed: 'blood',
}

const REMOTE_STATUS_LABELS: Record<string, string> = {
  pending: 'Pending',
  ok: 'Copied',
  failed: 'Failed',
}

export function BackupsTable({
  id,
  backups,
  isLoading,
  canManage,
  instanceState,
  retention,
  onRestore,
  onDelete,
}: {
  id: string
  backups: Backup[]
  isLoading: boolean
  canManage: boolean
  instanceState: InstanceState | undefined
  retention?: ReactNode
  onRestore: (backupId: number, stopIfRunning: boolean) => void
  onDelete: (backupId: number) => void
}) {
  const running = instanceState === 'running'
  const retryUpload = useRetryRemoteUpload(id)

  function confirmRestore(backup: Backup) {
    let stopIfRunning = running
    modals.openConfirmModal({
      title: 'Restore backup',
      children: (
        <Stack gap="sm">
          <Text size="sm">
            Restore <strong>{backup.filename}</strong>? A pre-restore backup of the current save is taken first. The
            instance must be stopped for the restore to run.
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
      onConfirm: () => onRestore(backup.id, stopIfRunning),
    })
  }

  function confirmDelete(backup: Backup) {
    modals.openConfirmModal({
      title: 'Delete backup',
      children: (
        <Text size="sm">
          Delete <strong>{backup.filename}</strong>? This cannot be undone.
        </Text>
      ),
      labels: { confirm: 'Delete', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => onDelete(backup.id),
    })
  }

  const columns: DataTableColumn<Backup>[] = [
    {
      key: 'created',
      header: 'Created',
      sortable: true,
      sortValue: (b) => b.created_at,
      render: (b) => (
        <>
          <Text size="sm">{fmtTime(b.created_at)}</Text>
          <Text size="xs" c="dimmed">
            {fmtAgo(b.created_at)}
          </Text>
        </>
      ),
    },
    {
      key: 'kind',
      header: 'Kind',
      render: (b) => (
        <Group gap="xs">
          <Badge color={BACKUP_KIND_COLORS[b.kind]} variant="light">
            {BACKUP_KIND_LABELS[b.kind]}
          </Badge>
          {b.missing && (
            <Badge color="red" variant="light">
              Missing
            </Badge>
          )}
        </Group>
      ),
    },
    { key: 'world', header: 'World', render: (b) => b.world },
    { key: 'size', header: 'Size', render: (b) => fmtBytes(b.size_bytes) },
    { key: 'note', header: 'Note', render: (b) => b.note || <Dash /> },
    {
      key: 'offsite',
      header: 'Off-site',
      render: (b) =>
        !b.remote_status ? (
          <Dash />
        ) : (
          <Group gap={4} wrap="nowrap">
            {b.remote_status === 'failed' && b.remote_error ? (
              <Tooltip label={b.remote_error} multiline maw={280}>
                <Badge
                  color={REMOTE_STATUS_COLORS[b.remote_status]}
                  variant="light"
                  tabIndex={0}
                  aria-label={`Off-site copy failed: ${b.remote_error}`}
                >
                  {REMOTE_STATUS_LABELS[b.remote_status]}
                </Badge>
              </Tooltip>
            ) : (
              <Badge color={REMOTE_STATUS_COLORS[b.remote_status]} variant="light">
                {REMOTE_STATUS_LABELS[b.remote_status]}
              </Badge>
            )}
            {canManage && b.remote_status === 'failed' && (
              <Tooltip label="Retry off-site copy">
                <ActionIcon
                  variant="subtle"
                  color="straw"
                  aria-label="Retry off-site copy"
                  loading={retryUpload.isPending && retryUpload.variables === b.id}
                  onClick={() => retryUpload.mutate(b.id)}
                >
                  <IconRefresh size={16} />
                </ActionIcon>
              </Tooltip>
            )}
          </Group>
        ),
    },
  ]

  return (
    <SectionCard title="Backups" description={retention} flush>
      <DataTable
        columns={columns}
        rows={backups}
        rowKey={(b) => b.id}
        loading={isLoading}
        defaultSort={{ key: 'created', dir: 'desc' }}
        minWidth={980}
        empty={<EmptyState icon={<IconDatabase size={22} />} title="No backups yet." description="Back up now, or upload one." />}
        actions={
          canManage
            ? (b) => (
                <Group gap={4} wrap="nowrap" justify="flex-end">
                  <Tooltip label="Download">
                    <ActionIcon
                      component="a"
                      href={api.url(`/instances/${id}/backups/${b.id}/download`)}
                      variant="subtle"
                      aria-label={`Download ${b.filename}`}
                      disabled={b.missing}
                    >
                      <IconDownload size={16} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Restore">
                    <ActionIcon
                      variant="subtle"
                      color="straw"
                      aria-label={`Restore ${b.filename}`}
                      disabled={b.missing}
                      onClick={() => confirmRestore(b)}
                    >
                      <IconRestore size={16} />
                    </ActionIcon>
                  </Tooltip>
                  <Tooltip label="Delete">
                    <ActionIcon variant="subtle" color="red" aria-label={`Delete ${b.filename}`} onClick={() => confirmDelete(b)}>
                      <IconTrash size={16} />
                    </ActionIcon>
                  </Tooltip>
                </Group>
              )
            : undefined
        }
      />
    </SectionCard>
  )
}
