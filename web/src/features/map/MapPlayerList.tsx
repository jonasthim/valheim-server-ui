// Player list shown under the map (U-A6a): live names from the agent status
// stream (or the map poll as a fallback — see MapTab), a dimmed "position
// hidden" tag for players who opted out of sharing their position on the
// map, and a Kick action for operators while the agent is connected.
// Precedent: features/instances/players/PlayersOnlinePanel.tsx.
import { Badge, Button, Group, Text } from '@mantine/core'
import { Link } from 'react-router-dom'
import type { AgentPlayer } from '../../api/types'
import { DataTable, EmptyState, SectionCard } from '../../ui'
import { openConfirmKick } from '../agent/openConfirmKick'

export function MapPlayerList({
  id,
  players,
  connected,
  onKick,
}: {
  id: string
  players: AgentPlayer[]
  connected: boolean
  /** Present when the caller may kick; the button also requires `connected`. */
  onKick?: (name: string) => void
}) {
  const canKick = !!onKick && connected

  return (
    <SectionCard
      title={`Players online (${players.length})`}
      flush
      actions={
        <Button size="xs" variant="subtle" component={Link} to={`/instances/${id}/players`}>
          All players
        </Button>
      }
    >
      <DataTable
        minWidth={320}
        columns={[
          {
            key: 'player',
            header: 'Player',
            render: (p) => (
              <Group gap={6} wrap="nowrap">
                <Text size="sm">{p.name}</Text>
                {(p.visible === false || !p.position) && (
                  <Badge size="sm" color="gray" variant="outline">
                    position hidden
                  </Badge>
                )}
              </Group>
            ),
          },
        ]}
        rows={players}
        rowKey={(p) => p.uid}
        empty={<EmptyState compact title="Nobody online." />}
        actions={
          canKick
            ? (p) => (
                <Button
                  size="compact-xs"
                  color="red"
                  variant="subtle"
                  onClick={() => openConfirmKick({ name: p.name, onConfirm: () => onKick?.(p.name) })}
                >
                  Kick
                </Button>
              )
            : undefined
        }
      />
    </SectionCard>
  )
}
