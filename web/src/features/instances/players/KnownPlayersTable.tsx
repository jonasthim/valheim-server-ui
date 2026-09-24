import { useState } from 'react'
import { ActionIcon, CopyButton, Group, Menu, Text, TextInput, Tooltip } from '@mantine/core'
import { IconBan, IconCheck, IconCopy, IconDots, IconPencil, IconShieldCheck, IconUserCheck } from '@tabler/icons-react'
import type { KnownPlayer, ListKind } from '../../../api/types'
import { fmtAgo } from '../../../lib/format'
import { DataTable, EmptyState, SectionCard } from '../../../ui'
import type { DataTableColumn } from '../../../ui'
import { LIST_KIND_ACTION_LABELS } from './constants'
import { useSetPlayerNote } from './usePlayers'

const LIST_ICONS: Record<ListKind, typeof IconShieldCheck> = {
  admin: IconShieldCheck,
  permitted: IconUserCheck,
  banned: IconBan,
}

/** "1h 23m" / "45m" / "<1m" / "0m" for a cumulative playtime in seconds. */
function fmtPlaytime(totalSeconds: number): string {
  if (!totalSeconds) return '0m'
  if (totalSeconds < 60) return '<1m'
  const totalMinutes = Math.floor(totalSeconds / 60)
  const hours = Math.floor(totalMinutes / 60)
  const minutes = totalMinutes % 60
  return hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`
}

/**
 * Inline-editable note cell. Viewers see plain text (or a dimmed dash);
 * operators can click the text or the pencil to edit, save on Enter/blur,
 * Escape to cancel.
 */
function NoteCell({
  playerId,
  note,
  canEdit,
  onSave,
}: {
  playerId: string
  note: string
  canEdit: boolean
  onSave: (note: string) => void
}) {
  const [editing, setEditing] = useState(false)
  const [value, setValue] = useState(note)

  if (!canEdit) {
    return note ? <Text size="sm">{note}</Text> : <Text c="dimmed">—</Text>
  }

  if (editing) {
    return (
      <TextInput
        size="xs"
        autoFocus
        maxLength={500}
        value={value}
        onChange={(e) => setValue(e.currentTarget.value)}
        onKeyDown={(e) => {
          if (e.key === 'Enter') e.currentTarget.blur()
          else if (e.key === 'Escape') setEditing(false)
        }}
        onBlur={() => {
          setEditing(false)
          if (value !== note) onSave(value)
        }}
        aria-label={`Note for ${playerId}`}
      />
    )
  }

  return (
    <Group
      gap={4}
      wrap="nowrap"
      style={{ cursor: 'pointer' }}
      onClick={() => {
        setValue(note)
        setEditing(true)
      }}
    >
      {note ? <Text size="sm">{note}</Text> : <Text c="dimmed">—</Text>}
      <ActionIcon
        size="md"
        variant="subtle"
        aria-label={`Edit note for ${playerId}`}
        onClick={(e) => {
          e.stopPropagation()
          setValue(note)
          setEditing(true)
        }}
      >
        <IconPencil size={14} />
      </ActionIcon>
    </Group>
  )
}

export function KnownPlayersTable({
  id,
  players,
  isLoading,
  canManageLists,
  onAddToList,
}: {
  id: string
  players: KnownPlayer[]
  isLoading: boolean
  canManageLists: boolean
  onAddToList: (kind: ListKind, playerId: string) => void
}) {
  const setNote = useSetPlayerNote(id)

  const columns: DataTableColumn<KnownPlayer>[] = [
    {
      key: 'name',
      header: 'Name',
      sticky: true,
      render: (p) => p.name || <Text c="dimmed">unknown</Text>,
    },
    {
      key: 'platform_id',
      header: 'Platform id',
      render: (p) => (
        <Group gap={4} wrap="nowrap">
          <Text ff="monospace" size="sm">
            {p.platform_id}
          </Text>
          <CopyButton value={p.platform_id}>
            {({ copied, copy }) => (
              <Tooltip label={copied ? 'Copied' : 'Copy platform id'}>
                <ActionIcon size="sm" variant="subtle" onClick={copy} aria-label={`Copy platform id ${p.platform_id}`}>
                  {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                </ActionIcon>
              </Tooltip>
            )}
          </CopyButton>
        </Group>
      ),
    },
    { key: 'first_seen', header: 'First seen', render: (p) => fmtAgo(p.first_seen_at) },
    {
      key: 'last_seen',
      header: 'Last seen',
      sortable: true,
      sortValue: (p) => p.last_seen_at,
      render: (p) => fmtAgo(p.last_seen_at),
    },
    { key: 'sessions', header: 'Sessions', render: (p) => p.session_count },
    { key: 'playtime', header: 'Playtime', render: (p) => fmtPlaytime(p.total_play_seconds) },
    {
      key: 'note',
      header: 'Note',
      render: (p) => (
        <NoteCell
          playerId={p.platform_id}
          note={p.note ?? ''}
          canEdit={canManageLists}
          onSave={(note) => setNote.mutate({ platformId: p.platform_id, note })}
        />
      ),
    },
  ]

  return (
    <SectionCard title="Known players" flush>
      <DataTable
        columns={columns}
        rows={players}
        rowKey={(p) => p.platform_id}
        loading={isLoading}
        defaultSort={{ key: 'last_seen', dir: 'desc' }}
        minWidth={640}
        empty={<EmptyState compact title="No players have connected to this instance yet." />}
        actions={
          canManageLists
            ? (p) => (
                <Menu withinPortal position="bottom-end">
                  <Menu.Target>
                    <ActionIcon variant="subtle" aria-label={`Actions for ${p.platform_id}`}>
                      <IconDots size={16} />
                    </ActionIcon>
                  </Menu.Target>
                  <Menu.Dropdown>
                    {(['admin', 'permitted', 'banned'] as ListKind[]).map((kind) => {
                      const Icon = LIST_ICONS[kind]
                      return (
                        <Menu.Item
                          key={kind}
                          leftSection={<Icon size={16} />}
                          color={kind === 'banned' ? 'red' : undefined}
                          onClick={() => onAddToList(kind, p.platform_id)}
                        >
                          {LIST_KIND_ACTION_LABELS[kind]}
                        </Menu.Item>
                      )
                    })}
                  </Menu.Dropdown>
                </Menu>
              )
            : undefined
        }
      />
    </SectionCard>
  )
}
