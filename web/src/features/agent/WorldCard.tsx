// Live world card on the Overview: day, clock, weather, players and global
// keys straight from the running server through the agent, with the save
// and broadcast actions.
import type { CSSProperties } from 'react'
import { Anchor, Badge, Button, Group, SimpleGrid, Stack, Text, Tooltip } from '@mantine/core'
import { Link } from 'react-router-dom'
import { IconDeviceFloppy, IconMoon, IconSun, IconSwords } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo } from '../../lib/format'
import { SectionCard, StatTile, StatusPill } from '../../ui'
import { AgentSetupNotice } from './AgentSetupNotice'
import { BroadcastButton } from './BroadcastButton'
import { ChatButton } from './ChatButton'
import { WorldControls } from './WorldControls'
import { fmtWorldTime, useAgentCommand } from './useAgent'
import { useAgentSetup } from './useAgentSetup'

const MAX_KEYS = 12

// Recolors every Mantine Paper/Text in the subtree to ink-on-parchment: Paper
// reads its background from --mantine-color-body, and Text (default or
// c="dimmed") either inherits `color` or reads --mantine-color-dimmed, so
// shadowing these on a wrapper covers SectionCard, its Text/Title children
// and the StatTile grid without editing those files. Mirrors the
// .emptyParchment treatment in ui.module.css, just scoped inline instead of
// via a CSS-module class (WorldCard.tsx is the only file this card lets us
// touch for this variant).
const PARCHMENT_VARS = {
  background: 'var(--vh-parchment)',
  color: 'var(--vh-ink)',
  '--mantine-color-body': 'var(--vh-parchment)',
  // index.css paints every Paper with --vh-surface, so shadow that too or the
  // card (and the StatTile papers inside it) stay timber-coloured.
  '--vh-surface': 'var(--vh-parchment)',
  '--vh-surface-2': 'var(--vh-parchment-2)',
  '--mantine-color-dimmed': 'var(--vh-ink)',
  '--vh-text-soft': 'var(--vh-ink)',
} as CSSProperties

export function WorldCard({ id, variant = 'default' }: { id: string; variant?: 'default' | 'parchment' }) {
  const { hasRole } = useAuth()
  const command = useAgentCommand(id)
  const { stage, info } = useAgentSetup(id)

  // Nothing to show until the agent is ready or merely offline; otherwise
  // the notice explains what's missing and offers the fix in place.
  if (stage !== 'ready' && stage !== 'offline') return <AgentSetupNotice id={id} context="overview" />
  if (!info) return null

  const canOperate = hasRole('operator')
  const st = info.connected ? info.status : undefined
  // The agent serializes empty world-state slices as JSON null; guard the
  // array reads so a world with no global keys (or no players) still renders.
  const globalKeys = st?.global_keys ?? []
  const players = st?.players ?? []
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
          {variant === 'parchment' ? (
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
              value={players.length}
              hint={
                players.filter((p) => p.position).length < players.length
                  ? `${players.length - players.filter((p) => p.position).length} hidden on map`
                  : 'positions known'
              }
              accent={players.length > 0 ? 'var(--vh-moss)' : undefined}
            />
          </SimpleGrid>
          {event && (
            <Group gap="xs" align="center">
              <IconSwords size={16} color="var(--vh-rust, #b4551d)" />
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

  if (variant !== 'parchment') return card
  return <div style={PARCHMENT_VARS}>{card}</div>
}
