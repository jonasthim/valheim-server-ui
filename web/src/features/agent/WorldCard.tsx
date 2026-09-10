// Live world card on the Overview: day, clock, weather, players and global
// keys straight from the running server through the agent, with the save
// and broadcast actions.
import { Badge, Button, Group, SimpleGrid, Stack, Text, Tooltip } from '@mantine/core'
import { IconDeviceFloppy, IconMoon, IconSun } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo } from '../../lib/format'
import { SectionCard, StatTile, StatusPill } from '../../ui'
import { BroadcastButton } from './BroadcastButton'
import { fmtWorldTime, useAgent, useAgentCommand } from './useAgent'

const MAX_KEYS = 12

export function WorldCard({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const agent = useAgent(id)
  const command = useAgentCommand(id)
  const info = agent.data

  // Nothing to show until the plugin exists; the Mods tab explains how.
  if (!info?.installed) return null

  const canOperate = hasRole('operator')
  const st = info.connected ? info.status : undefined

  return (
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
            </>
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
          <SimpleGrid cols={{ base: 2, md: 4 }}>
            <StatTile label="Day" value={st.world.day} hint={st.world.name} />
            <StatTile
              label="Time"
              value={fmtWorldTime(st.world.day_fraction)}
              hint={st.world.is_night ? 'night' : 'day'}
              icon={st.world.is_night ? <IconMoon size={16} /> : <IconSun size={16} />}
            />
            <StatTile label="Weather" value={st.world.weather || '—'} hint="current environment" />
            <StatTile
              label="Players"
              value={st.players.length}
              hint={
                st.players.filter((p) => p.position).length < st.players.length
                  ? `${st.players.length - st.players.filter((p) => p.position).length} hidden on map`
                  : 'positions known'
              }
              accent={st.players.length > 0 ? 'var(--vh-moss)' : undefined}
            />
          </SimpleGrid>
          <div>
            <Text size="xs" c="dimmed" mb={4}>
              World keys ({st.global_keys.length})
            </Text>
            <Group gap={6}>
              {st.global_keys.length === 0 && (
                <Text size="sm" c="dimmed">
                  none yet
                </Text>
              )}
              {st.global_keys.slice(0, MAX_KEYS).map((k) => (
                <Badge key={k} variant="light" color="straw" size="sm">
                  {k}
                </Badge>
              ))}
              {st.global_keys.length > MAX_KEYS && (
                <Badge variant="outline" color="gray" size="sm">
                  +{st.global_keys.length - MAX_KEYS} more
                </Badge>
              )}
            </Group>
          </div>
          <Text size="xs" c="dimmed">
            Agent v{st.agent_version}
            {st.game_version ? ` · game ${st.game_version}` : ''}
            {info.last_seen ? ` · updated ${fmtAgo(info.last_seen)}` : ''}
          </Text>
        </Stack>
      )}
    </SectionCard>
  )
}
