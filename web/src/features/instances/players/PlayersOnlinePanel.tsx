import { Badge, Button, Group, Text } from '@mantine/core'
import { IconUserOff } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import type { OnlinePlayer, PlayersResponse } from '../../../api/types'
import { fmtAgo } from '../../../lib/format'
import { Dash, DataTable, EmptyState, SectionCard, StatusPill } from '../../../ui'
import type { DataTableColumn } from '../../../ui'
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

  const columns: DataTableColumn<OnlinePlayer>[] = [
    { key: 'name', header: 'Name', render: (p) => p.name },
    {
      key: 'platform_id',
      header: 'Platform id',
      render: (p) =>
        p.platform_id ? (
          <Text ff="monospace" size="sm">
            {p.platform_id}
          </Text>
        ) : (
          <Dash />
        ),
    },
    { key: 'connected', header: 'Connected', render: (p) => fmtAgo(p.connected_at) },
  ]

  return (
    <SectionCard
      title="Online"
      flush
      actions={
        <Group gap="xs" wrap="wrap" justify="flex-end">
          <StatusPill color={count > 0 ? 'moss' : 'gray'}>{count} online</StatusPill>
          <ChatButton id={instanceId} disabled={!agentConnected} canSay={!!canOperate} />
          {canOperate && <BroadcastButton id={instanceId} disabled={!agentConnected} />}
          <Button size="xs" variant="subtle" component={Link} to={`/instances/${instanceId}/map`}>
            Map
          </Button>
        </Group>
      }
    >
      <DataTable
        minWidth={480}
        columns={columns}
        rows={online}
        rowKey={(p, i) => p.platform_id ?? `${p.name}-${i}`}
        loading={isLoading}
        empty={<EmptyState compact title="No players are connected right now." description="Players appear here as they join." />}
        actions={
          onKick
            ? (p) => (
                <Button
                  size="compact-xs"
                  variant="subtle"
                  color="red"
                  leftSection={<IconUserOff size={14} />}
                  disabled={kickPending}
                  onClick={() => openConfirmKick({ name: p.name, onConfirm: () => onKick(p) })}
                >
                  Kick
                </Button>
              )
            : undefined
        }
      />

      <Group gap="xs" p="lg" pt={0}>
        <Badge size="sm" color="gray" variant="outline">
          {COUNT_SOURCE_LABELS[source]}
        </Badge>
        <Text size="xs" c="dimmed">
          Player names come from the console log (best effort); the count above comes {COUNT_SOURCE_LABELS[source]}.
        </Text>
      </Group>
    </SectionCard>
  )
}
