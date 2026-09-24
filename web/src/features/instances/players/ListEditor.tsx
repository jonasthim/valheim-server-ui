import { useState } from 'react'
import { ActionIcon, Button, Group, Skeleton, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import type { ListKind } from '../../../api/types'
import { Dash, LoadError } from '../../../ui'
import { usePlayerList, useSavePlayerList } from './usePlayers'
import { PLATFORM_ID_PATTERN } from './constants'
import dt from '../../../ui/DataTable.module.css'

/** Kind-specific confirm-dialog copy for removing one entry from a list. */
const REMOVE_CONFIRM_COPY: Record<
  ListKind,
  {
    title: string
    body: (id: string) => string
    labels: { confirm: string; cancel: string }
    confirmProps?: { color: string }
  }
> = {
  banned: {
    title: 'Unban player',
    body: (id) => `Unban ${id}? They can rejoin immediately.`,
    labels: { confirm: 'Unban', cancel: 'Cancel' },
    confirmProps: { color: 'red' },
  },
  admin: {
    title: 'Remove admin',
    body: (id) => `Remove admin rights from ${id}?`,
    labels: { confirm: 'Remove', cancel: 'Cancel' },
  },
  permitted: {
    title: 'Remove from permitted list',
    body: (id) => `Remove ${id} from the permitted list? While the list is non-empty, only listed players can join.`,
    labels: { confirm: 'Remove', cancel: 'Cancel' },
  },
}

/** Table + inline add row for one of the three list files. Read-only for viewers. */
export function ListEditor({ id, kind, canEdit }: { id: string; kind: ListKind; canEdit: boolean }) {
  const query = usePlayerList(id, kind)
  const save = useSavePlayerList(id, kind)
  const [newId, setNewId] = useState('')
  const [newComment, setNewComment] = useState('')
  const [idError, setIdError] = useState<string | null>(null)

  const entries = query.data?.entries ?? []

  function addEntry() {
    const trimmed = newId.trim()
    if (!PLATFORM_ID_PATTERN.test(trimmed)) {
      setIdError('Letters, digits and underscores only, 1-64 characters')
      return
    }
    if (entries.some((e) => e.id === trimmed)) {
      setIdError('Already in this list')
      return
    }
    setIdError(null)
    save.mutate(
      { kind, entries: [...entries, { id: trimmed, comment: newComment.trim() || undefined }] },
      {
        onSuccess: () => {
          setNewId('')
          setNewComment('')
        },
      },
    )
  }

  function removeEntry(entryId: string) {
    const copy = REMOVE_CONFIRM_COPY[kind]
    modals.openConfirmModal({
      title: copy.title,
      children: <Text size="sm">{copy.body(entryId)}</Text>,
      labels: copy.labels,
      confirmProps: copy.confirmProps,
      onConfirm: () => save.mutate({ kind, entries: entries.filter((e) => e.id !== entryId) }),
    })
  }

  return (
    <Stack gap="sm">
      {query.isLoading && <Skeleton height={100} />}

      {query.isError && (
        <LoadError error={query.error} title="Could not load the list" onRetry={() => query.refetch()} />
      )}

      {!query.isLoading && !query.isError && (
        <Table.ScrollContainer minWidth={480}>
          <Table verticalSpacing="xs" classNames={{ th: dt.th, td: dt.td, tr: dt.tr, table: dt.table }}>
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Platform id</Table.Th>
                <Table.Th>Comment</Table.Th>
                {canEdit && <Table.Th />}
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {entries.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={canEdit ? 3 : 2}>
                    <Text c="dimmed" size="sm" py="xs">
                      Empty.
                    </Text>
                  </Table.Td>
                </Table.Tr>
              )}
              {entries.map((e) => (
                <Table.Tr key={e.id}>
                  <Table.Td>
                    <Text ff="monospace" size="sm">
                      {e.id}
                    </Text>
                  </Table.Td>
                  <Table.Td>{e.comment || <Dash />}</Table.Td>
                  {canEdit && (
                    <Table.Td>
                      <Tooltip label="Remove">
                        <ActionIcon
                          variant="subtle"
                          color="red"
                          aria-label={`Remove ${e.id}`}
                          onClick={() => removeEntry(e.id)}
                          loading={save.isPending}
                        >
                          <IconTrash size={16} />
                        </ActionIcon>
                      </Tooltip>
                    </Table.Td>
                  )}
                </Table.Tr>
              ))}
              {canEdit && (
                <Table.Tr>
                  <Table.Td>
                    <TextInput
                      size="xs"
                      placeholder="Platform id"
                      value={newId}
                      error={idError}
                      onChange={(e) => {
                        setNewId(e.currentTarget.value)
                        setIdError(null)
                      }}
                      onKeyDown={(e) => e.key === 'Enter' && addEntry()}
                      aria-label="New platform id"
                    />
                  </Table.Td>
                  <Table.Td>
                    <TextInput
                      size="xs"
                      placeholder="Comment (optional)"
                      value={newComment}
                      onChange={(e) => setNewComment(e.currentTarget.value)}
                      onKeyDown={(e) => e.key === 'Enter' && addEntry()}
                      aria-label="New entry comment"
                    />
                  </Table.Td>
                  <Table.Td>
                    <Button
                      size="xs"
                      variant="light"
                      leftSection={<IconPlus size={14} />}
                      onClick={addEntry}
                      loading={save.isPending}
                      disabled={!newId.trim() || !query.isSuccess}
                    >
                      Add
                    </Button>
                  </Table.Td>
                </Table.Tr>
              )}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}

      <Group gap={4}>
        <Text size="xs" c="dimmed">
          Valheim reloads these files live; the permitted list, when non-empty, restricts who can join.
        </Text>
      </Group>
    </Stack>
  )
}
