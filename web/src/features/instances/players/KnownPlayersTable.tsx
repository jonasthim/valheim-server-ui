import { ActionIcon, CopyButton, Group, Menu, Skeleton, Table, Text, Tooltip } from '@mantine/core'
import { IconBan, IconCheck, IconCopy, IconDots, IconShieldCheck, IconUserCheck } from '@tabler/icons-react'
import type { KnownPlayer, ListKind } from '../../../api/types'
import { fmtAgo } from '../../../lib/format'
import { SectionCard } from '../../../ui'
import { LIST_KIND_ACTION_LABELS } from './constants'
import classes from './players.module.css'

const LIST_ICONS: Record<ListKind, typeof IconShieldCheck> = {
  admin: IconShieldCheck,
  permitted: IconUserCheck,
  banned: IconBan,
}

export function KnownPlayersTable({
  players,
  isLoading,
  canManageLists,
  onAddToList,
}: {
  players: KnownPlayer[]
  isLoading: boolean
  canManageLists: boolean
  onAddToList: (kind: ListKind, playerId: string) => void
}) {
  const sorted = [...players].sort((a, b) => (a.last_seen_at < b.last_seen_at ? 1 : -1))

  return (
    <SectionCard title="Known players" flush>
      {isLoading && (
        <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
          <Skeleton height={80} />
        </div>
      )}

      {!isLoading && sorted.length === 0 && (
        <Text c="dimmed" size="sm" p="lg">
          No players have connected to this instance yet.
        </Text>
      )}

      {!isLoading && sorted.length > 0 && (
        <Table.ScrollContainer minWidth={640}>
          <Table verticalSpacing="xs" highlightOnHover className={classes.stickyNameTable}>
            <Table.Thead>
              <Table.Tr>
                <Table.Th className={classes.stickyNameCol}>Name</Table.Th>
                <Table.Th>Platform id</Table.Th>
                <Table.Th>First seen</Table.Th>
                <Table.Th>Last seen</Table.Th>
                <Table.Th>Sessions</Table.Th>
                {canManageLists && <Table.Th />}
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {sorted.map((p: KnownPlayer) => (
                <Table.Tr key={p.platform_id}>
                  <Table.Td className={classes.stickyNameCol}>{p.name || <Text c="dimmed">unknown</Text>}</Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap">
                      <Text ff="monospace" size="sm">
                        {p.platform_id}
                      </Text>
                      <CopyButton value={p.platform_id}>
                        {({ copied, copy }) => (
                          <Tooltip label={copied ? 'Copied' : 'Copy platform id'}>
                            <ActionIcon
                              size="sm"
                              variant="subtle"
                              onClick={copy}
                              aria-label={`Copy platform id ${p.platform_id}`}
                            >
                              {copied ? <IconCheck size={14} /> : <IconCopy size={14} />}
                            </ActionIcon>
                          </Tooltip>
                        )}
                      </CopyButton>
                    </Group>
                  </Table.Td>
                  <Table.Td>{fmtAgo(p.first_seen_at)}</Table.Td>
                  <Table.Td>{fmtAgo(p.last_seen_at)}</Table.Td>
                  <Table.Td>{p.session_count}</Table.Td>
                  {canManageLists && (
                    <Table.Td>
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
                    </Table.Td>
                  )}
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </SectionCard>
  )
}
