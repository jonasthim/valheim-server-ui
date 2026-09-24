// Proactive notice for the agent's setup state, mounted wherever the
// agent's absence is what the user is looking at (the Overview world card,
// the map, the players tab) instead of a text link telling them to go find
// the Mods tab themselves. Hidden once the agent is ready, merely offline,
// or an install job is already under way.
import { Alert, Button, Group, Text } from '@mantine/core'
import { IconPlugConnected } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { openConfirmInstallBepinex } from '../mods/openConfirmInstallBepinex'
import { useInstallBepinex, useSetBepinexEnabled } from '../mods/useMods'
import { openConfirmInstallAgent } from './openConfirmInstallAgent'
import { useInstallAgent } from './useAgent'
import { useAgentSetup } from './useAgentSetup'

const BODY_BY_CONTEXT: Record<'overview' | 'map' | 'players', string> = {
  overview: 'The agent shows the live world here: day, weather, players, and lets you save, broadcast and kick.',
  map: 'The agent renders the live map with fog of war, players and pins.',
  players: 'The agent shows live positions and lets you kick players.',
}

export function AgentSetupNotice({
  id,
  context,
  compact,
}: {
  id: string
  context: 'overview' | 'map' | 'players'
  compact?: boolean
}) {
  const { hasRole } = useAuth()
  const { stage, isRunning, activeInstallJob, info } = useAgentSetup(id)
  const installAgent = useInstallAgent(id)
  const installBepinex = useInstallBepinex(id)
  const setBepinexEnabled = useSetBepinexEnabled(id)

  if (stage === 'ready' || stage === 'offline' || activeInstallJob) return null

  let title: string
  if (stage === 'bepinex') title = 'Install BepInEx and the Valheim UI Agent'
  else if (stage === 'install') title = 'Install the Valheim UI Agent'
  else if (stage === 'update') title = 'Update the Valheim UI Agent'
  else title = 'BepInEx is disabled for this instance'

  let body: string
  if (stage === 'update') {
    body = `A newer agent ships with this manager version (bundled v${info?.bundled_version}, installed v${info?.installed_version}). Updating restarts the server briefly.`
  } else if (stage === 'disabled') {
    body = 'The agent is installed but BepInEx is switched off, so it cannot load. Enable BepInEx to use it.'
  } else {
    body = BODY_BY_CONTEXT[context]
  }

  let actionLabel: string
  let actionLoading: boolean
  let onAction: () => void
  if (stage === 'bepinex') {
    actionLabel = 'Install BepInEx'
    actionLoading = installBepinex.isPending
    onAction = () =>
      openConfirmInstallBepinex({
        upgrade: false,
        isRunning,
        onConfirm: (stop) => installBepinex.mutate({ stop_if_running: stop }),
      })
  } else if (stage === 'disabled') {
    actionLabel = 'Enable BepInEx'
    actionLoading = setBepinexEnabled.isPending
    onAction = () => setBepinexEnabled.mutate(true)
  } else {
    actionLabel = stage === 'update' ? 'Update agent' : 'Install agent'
    actionLoading = installAgent.isPending
    onAction = () =>
      openConfirmInstallAgent({
        update: stage === 'update',
        isRunning,
        onConfirm: (stop) => installAgent.mutate({ stop_if_running: stop }),
      })
  }

  return (
    <Alert
      variant="light"
      color="frost"
      icon={<IconPlugConnected size={16} />}
      title={title}
      p={compact ? 'xs' : undefined}
    >
      <Group justify="space-between" align="center" wrap="wrap">
        <Text size="sm">{body}</Text>
        {hasRole('operator') && (
          <Group gap="xs">
            <Button size="xs" variant="light" loading={actionLoading} onClick={onAction}>
              {actionLabel}
            </Button>
          </Group>
        )}
      </Group>
    </Alert>
  )
}
