import { useState } from 'react'
import { ActionIcon, Button, Group, Skeleton, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core'
import { IconPlus, IconTrash } from '@tabler/icons-react'
import type { ListKind } from '../../../api/types'
import { usePlayerList, useSavePlayerList } from './usePlayers'
import { PLATFORM_ID_PATTERN } from './constants'

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
    save.mutate({ kind, entries: entries.filter((e) => e.id !== entryId) })
  }

  return (
    <Stack gap="sm">
      {query.isLoading && <Skeleton height={100} />}

      {!query.isLoading && (
        <Table.ScrollContainer minWidth={480}>
          <Table verticalSpacing="xs">
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
                  <Table.Td>{e.comment || '-'}</Table.Td>
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
                      disabled={!newId.trim()}
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
