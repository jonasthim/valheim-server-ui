// Status card for the Valheim UI Agent on the Mods tab: installed/connected
// state, version against the bundled one, and the install/update job.
import { Badge, Button, Group, Skeleton, Stack, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconPlugConnected } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { SectionCard, StatusPill } from '../../ui'
import { useInstance } from '../instances/useInstance'
import { useModsOverview } from '../mods/useMods'
import { useAgent, useInstallAgent } from './useAgent'

export function AgentCard({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const agent = useAgent(id)
  const instance = useInstance(id)
  const mods = useModsOverview(id)
  const install = useInstallAgent(id)

  const isRunning = ['running', 'starting', 'stopping'].includes(instance.data?.status.state ?? '')
  const bepinexInstalled = !!mods.data?.bepinex.installed
  const info = agent.data

  function confirmInstall(update: boolean) {
    const verb = update ? 'Update' : 'Install'
    modals.openConfirmModal({
      title: `${verb} Valheim UI Agent`,
      children: (
        <Stack gap="xs">
          <span>
            The agent is installed while the instance is <strong>stopped</strong>; the game keeps plugin files open
            while it runs.
          </span>
          {isRunning && (
            <span style={{ color: 'var(--vh-text-soft)' }}>
              This instance is running. Confirming will stop it, {verb.toLowerCase()} the agent, and start it again.
            </span>
          )}
        </Stack>
      ),
      labels: { confirm: isRunning ? `Stop, ${verb.toLowerCase()} and start again` : verb, cancel: 'Cancel' },
      onConfirm: () => install.mutate({ stop_if_running: isRunning }),
    })
  }

  if (agent.isLoading) {
    return (
      <SectionCard title="Valheim UI Agent">
        <Skeleton height={48} />
      </SectionCard>
    )
  }

  let state: { color: string; label: string }
  if (!info?.installed) state = { color: 'gray', label: 'not installed' }
  else if (info.connected) state = { color: 'moss', label: `connected · v${info.installed_version ?? '?'}` }
  else if (!info.enabled) state = { color: 'gray', label: `installed v${info.installed_version ?? '?'} · BepInEx disabled` }
  else if (isRunning) state = { color: 'orange', label: `installed v${info.installed_version ?? '?'} · not reachable` }
  else state = { color: 'gray', label: `installed v${info.installed_version ?? '?'} · server stopped` }

  return (
    <SectionCard
      title="Valheim UI Agent"
      description="The manager's own server plugin: live players and world state on the Overview, kick and broadcast from the UI. Installed together with BepInEx; players need nothing."
      actions={
        <Group gap="sm" wrap="wrap" justify="flex-end">
          <StatusPill color={state.color}>
            <Group gap={6} wrap="nowrap">
              <IconPlugConnected size={14} />
              <span>{state.label}</span>
            </Group>
          </StatusPill>
          {info?.update_available && (
            <Badge color="frost" variant="light">
              bundled v{info.bundled_version}
            </Badge>
          )}
          {hasRole('operator') && bepinexInstalled && (!info?.installed || info.update_available) && (
            <Button size="xs" variant="light" loading={install.isPending} onClick={() => confirmInstall(!!info?.installed)}>
              {info?.installed ? 'Update agent' : 'Install agent'}
            </Button>
          )}
        </Group>
      }
    >
      {!bepinexInstalled && (
        <Text size="sm" c="dimmed">
          Install BepInEx above; the agent comes with it.
        </Text>
      )}
      {bepinexInstalled && info?.installed && !info.connected && info.last_error && isRunning && (
        <Text size="xs" c="dimmed">
          Last error: {info.last_error}
        </Text>
      )}
      {bepinexInstalled && info?.installed && info.connected && info.status && (
        <Text size="sm" c="dimmed">
          Agent v{info.status.agent_version} on game {info.status.game_version || '?'}, {info.status.players.length}{' '}
          player{info.status.players.length === 1 ? '' : 's'} online.
        </Text>
      )}
    </SectionCard>
  )
}
