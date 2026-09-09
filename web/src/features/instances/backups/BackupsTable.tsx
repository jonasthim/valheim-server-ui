import type { ReactNode } from 'react'
import { ActionIcon, Badge, Checkbox, Group, Skeleton, Stack, Table, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconDatabase, IconDownload, IconRestore, IconTrash } from '@tabler/icons-react'
import { api } from '../../../api/client'
import type { Backup, InstanceState } from '../../../api/types'
import { fmtAgo, fmtBytes, fmtTime } from '../../../lib/format'
import { EmptyState, SectionCard } from '../../../ui'
import { BACKUP_KIND_COLORS, BACKUP_KIND_LABELS } from './constants'

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

  const sorted = [...backups].sort((a, b) => (a.created_at < b.created_at ? 1 : -1))

  return (
    <SectionCard title="Backups" description={retention} flush>
      {isLoading ? (
        <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
          <Skeleton height={160} />
        </div>
      ) : backups.length === 0 ? (
        <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
          <EmptyState icon={<IconDatabase size={22} />} title="No backups yet." />
        </div>
      ) : (
        <Table.ScrollContainer minWidth={860}>
          <Table verticalSpacing="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Created</Table.Th>
                <Table.Th>Kind</Table.Th>
                <Table.Th>World</Table.Th>
                <Table.Th>Size</Table.Th>
                <Table.Th>Note</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {sorted.map((b) => (
                <Table.Tr key={b.id}>
                  <Table.Td>
                    <Text size="sm">{fmtTime(b.created_at)}</Text>
                    <Text size="xs" c="dimmed">
                      {fmtAgo(b.created_at)}
                    </Text>
                  </Table.Td>
                  <Table.Td>
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
                  </Table.Td>
                  <Table.Td>{b.world}</Table.Td>
                  <Table.Td>{fmtBytes(b.size_bytes)}</Table.Td>
                  <Table.Td>{b.note || '-'}</Table.Td>
                  <Table.Td>
                    {canManage && (
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
                          <ActionIcon
                            variant="subtle"
                            color="red"
                            aria-label={`Delete ${b.filename}`}
                            onClick={() => confirmDelete(b)}
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
      )}
    </SectionCard>
  )
}
