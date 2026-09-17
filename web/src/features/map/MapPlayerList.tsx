// Player list shown under the map (U-A6a): live names from the agent status
// stream (or the map poll as a fallback — see MapTab), a dimmed "position
// hidden" tag for players who opted out of sharing their position on the
// map, and a Kick action for operators while the agent is connected.
// Precedent: features/instances/players/PlayersOnlinePanel.tsx.
import { Badge, Button, Group, Table, Text } from '@mantine/core'
import { Link } from 'react-router-dom'
import type { AgentPlayer } from '../../api/types'
import { SectionCard } from '../../ui'
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
      {players.length === 0 && (
        <Text c="dimmed" size="sm" p="lg">
          Nobody online.
        </Text>
      )}

      {players.length > 0 && (
        <Table verticalSpacing="xs">
          <Table.Tbody>
            {players.map((p) => (
              <Table.Tr key={p.uid}>
                <Table.Td>
                  <Group gap={6} wrap="nowrap">
                    <Text size="sm">{p.name}</Text>
                    {(p.visible === false || !p.position) && (
                      <Badge size="sm" color="gray" variant="outline">
                        position hidden
                      </Badge>
                    )}
                  </Group>
                </Table.Td>
                {canKick && (
                  <Table.Td align="right">
                    <Button
                      size="xs"
                      color="red"
                      variant="subtle"
                      onClick={() => openConfirmKick({ name: p.name, onConfirm: () => onKick?.(p.name) })}
                    >
                      Kick
                    </Button>
                  </Table.Td>
                )}
              </Table.Tr>
            ))}
          </Table.Tbody>
        </Table>
      )}
    </SectionCard>
  )
}
