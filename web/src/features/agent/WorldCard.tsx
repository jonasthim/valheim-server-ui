// Live world card on the Overview: day, clock, weather, players and global
// keys straight from the running server through the agent, with the save
// and broadcast actions.
import { Anchor, Badge, Button, Group, Stack, Text, Tooltip } from '@mantine/core'
import { Link } from 'react-router-dom'
import { IconDeviceFloppy, IconMoon, IconSun, IconSwords } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo } from '../../lib/format'
import { SectionCard, StatStrip, StatusPill } from '../../ui'
import { BroadcastButton } from './BroadcastButton'
import { ChatButton } from './ChatButton'
import { WorldControls } from './WorldControls'
import { fmtWorldTime, useAgentCommand } from './useAgent'
import { useAgentSetup } from './useAgentSetup'

const MAX_KEYS = 12

export function WorldCard({ id, playersLink = false }: { id: string; playersLink?: boolean }) {
  const { hasRole } = useAuth()
  const command = useAgentCommand(id)
  const { stage, info } = useAgentSetup(id)

  // Nothing to show until the agent is ready or merely offline; otherwise
  // the notice explains what's missing and offers the fix in place.
  // The setup prompt is the Overview's full-width banner (OverviewTab), not
  // this card's job: with an agent installed the live world stays visible
  // even while an update is pending or the agent is briefly offline.
  if (stage === 'bepinex' || stage === 'install' || stage === 'disabled') return null
  if (!info) return null

  const canOperate = hasRole('operator')
  const st = info.connected ? info.status : undefined
  // The agent serializes empty world-state slices as JSON null; guard the
  // array reads so a world with no global keys (or no players) still renders.
  const globalKeys = st?.global_keys ?? []
  const players = st?.players ?? []
  const hiddenCount = players.length - players.filter((p) => p.position).length
  const modifiers = Object.entries(st?.modifiers ?? {})
  const event = st?.world?.event

  const card = (
    <SectionCard
      title="World"
      description="Live from the server through the Valheim UI Agent."
      actions={
        <Group gap="sm" wrap="wrap" justify="flex-end">
          <StatusPill color={info.connected ? 'moss' : 'gray'}>
            {info.connected ? 'agent connected' : 'agent offline'}
          </StatusPill>
          {canOperate && (
            <>
              <Tooltip label="Ask the server to write the world to disk now">
                <Button
                  size="xs"
                  variant="light"
                  leftSection={<IconDeviceFloppy size={14} />}
                  loading={command.isPending && command.variables?.command === 'save'}
                  disabled={!info.connected}
                  onClick={() => command.mutate({ command: 'save' })}
                >
                  Save world
                </Button>
              </Tooltip>
              <BroadcastButton id={id} disabled={!info.connected} />
              <WorldControls id={id} status={st} disabled={!info.connected} />
            </>
          )}
          {playersLink ? (
            <Anchor component={Link} to={`/instances/${id}/players`} size="sm">
              Players &amp; chat
            </Anchor>
          ) : (
            <ChatButton id={id} disabled={!info.connected} canSay={canOperate} />
          )}
        </Group>
      }
    >
      {!st && (
        <Text size="sm" c="dimmed">
          {!info.enabled
            ? 'BepInEx is disabled for this instance, so the agent is not loaded.'
            : info.last_error
              ? `The agent is not reachable: ${info.last_error}`
              : 'The agent reports once the server is running and the world has loaded.'}
        </Text>
      )}
      {st && (
        <Stack gap="md">
          <StatStrip
            cols={2}
            minCellWidth={110}
            items={[
              { label: 'Day', value: st.world.day, hint: st.world.name },
              {
                label: 'Time',
                value: fmtWorldTime(st.world.day_fraction),
                hint: st.world.is_night ? 'night' : 'day',
                icon: st.world.is_night ? <IconMoon size={14} /> : <IconSun size={14} />,
              },
              { label: 'Weather', value: st.world.weather || '—', hint: 'current environment' },
              {
                label: 'Players',
                value: players.length,
                hint: hiddenCount > 0 ? `${hiddenCount} hidden on map` : 'positions known',
                tone: players.length > 0 ? 'success' : 'default',
              },
            ]}
          />
          {event && (
            <Group gap="xs" align="center">
              <IconSwords size={16} color="var(--vh-text-soft)" />
              <Text size="sm" fw={600}>
                Event: {event.name}
              </Text>
              <Text size="sm" c="dimmed">
                {Math.max(0, Math.round(event.remaining_seconds / 60))} min remaining
              </Text>
            </Group>
          )}
          {modifiers.length > 0 && (
            <div>
              <Text size="xs" c="dimmed" mb={4}>
                World modifiers
              </Text>
              <Group gap={6}>
                {modifiers.map(([k, v]) => (
                  <Badge key={k} variant="outline" color="gray" size="sm">
                    {k}: {v}
                  </Badge>
                ))}
              </Group>
            </div>
          )}
          <div>
            <Text size="xs" c="dimmed" mb={4}>
              World keys ({globalKeys.length})
            </Text>
            <Group gap={6}>
              {globalKeys.length === 0 && (
                <Text size="sm" c="dimmed">
                  none yet
                </Text>
              )}
              {globalKeys.slice(0, MAX_KEYS).map((k) => (
                <Badge key={k} variant="light" color="straw" size="sm">
                  {k}
                </Badge>
              ))}
              {globalKeys.length > MAX_KEYS && (
                <Badge variant="outline" color="gray" size="sm">
                  +{globalKeys.length - MAX_KEYS} more
                </Badge>
              )}
            </Group>
          </div>
          <Text size="xs" c="dimmed">
            Agent v{st.agent_version}
            {st.game_version ? ` on game ${st.game_version}` : ''}
            {info.last_seen ? `, updated ${fmtAgo(info.last_seen)}` : ''}
          </Text>
        </Stack>
      )}
    </SectionCard>
  )

  return card
}
