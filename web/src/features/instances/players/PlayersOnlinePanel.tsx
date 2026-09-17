import { Badge, Button, Group, Skeleton, Stack, Table, Text } from '@mantine/core'
import { IconUserOff } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import type { OnlinePlayer, PlayersResponse } from '../../../api/types'
import { fmtAgo } from '../../../lib/format'
import { EmptyState, SectionCard, StatusPill } from '../../../ui'
import { BroadcastButton, ChatButton } from '../../agent'
import { openConfirmKick } from '../../agent/openConfirmKick'
import { COUNT_SOURCE_LABELS } from './constants'

export function PlayersOnlinePanel({
  data,
  isLoading,
  onKick,
  kickPending = false,
  instanceId,
  agentConnected,
  canOperate,
}: {
  data: PlayersResponse | undefined
  isLoading: boolean
  /** Present when the agent is connected and the user may kick. */
  onKick?: (player: OnlinePlayer) => void
  kickPending?: boolean
  instanceId: string
  /** Whether the agent is connected; gates Chat sending and enables Broadcast/Chat. */
  agentConnected?: boolean
  /** Whether the caller has the operator role; gates Broadcast and in-chat sending. */
  canOperate?: boolean
}) {
  const online = data?.online ?? []
  const count = data?.online_count ?? online.length
  const source = data?.count_source ?? 'none'

  return (
    <SectionCard
      title="Online"
      flush
      actions={
        <Group gap="xs">
          <StatusPill color={count > 0 ? 'moss' : 'gray'}>{count} online</StatusPill>
          <Badge size="sm" color="gray" variant="outline">
            {COUNT_SOURCE_LABELS[source]}
          </Badge>
          <ChatButton id={instanceId} disabled={!agentConnected} canSay={!!canOperate} />
          {canOperate && <BroadcastButton id={instanceId} disabled={!agentConnected} />}
          <Button size="xs" variant="subtle" component={Link} to={`/instances/${instanceId}/map`}>
            Map
          </Button>
        </Group>
      }
    >
      {isLoading && (
        <div style={{ padding: 'var(--mantine-spacing-lg)' }}>
          <Skeleton height={60} />
        </div>
      )}

      {!isLoading && online.length === 0 && (
        <EmptyState compact title="No players are connected right now." description="Players appear here as they join." />
      )}

      {!isLoading && online.length > 0 && (
        <Table.ScrollContainer minWidth={480}>
          <Table verticalSpacing="xs">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Name</Table.Th>
                <Table.Th>Platform id</Table.Th>
                <Table.Th>Connected</Table.Th>
                {onKick && <Table.Th />}
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {online.map((p, i) => (
                <Table.Tr key={p.platform_id ?? `${p.name}-${i}`}>
                  <Table.Td>{p.name}</Table.Td>
                  <Table.Td>
                    <Text ff="monospace" size="sm">
                      {p.platform_id ?? '-'}
                    </Text>
                  </Table.Td>
                  <Table.Td>{fmtAgo(p.connected_at)}</Table.Td>
                  {onKick && (
                    <Table.Td align="right">
                      <Button
                        size="compact-xs"
                        variant="subtle"
                        color="red"
                        leftSection={<IconUserOff size={14} />}
                        disabled={kickPending}
                        onClick={() => openConfirmKick({ name: p.name, onConfirm: () => onKick?.(p) })}
                      >
                        Kick
                      </Button>
                    </Table.Td>
                  )}
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}

      <Stack gap={2} p="lg" pt={online.length > 0 ? 0 : undefined}>
        <Text size="xs" c="dimmed">
          Player names come from the console log (best effort); the count above comes {COUNT_SOURCE_LABELS[source]}.
        </Text>
      </Stack>
    </SectionCard>
  )
}
