// Mod loader status card: BepInEx and the Valheim UI Agent as two rows of
// one card (installed state, enable toggle, install/upgrade job), plus the
// pending-restart and agent-status notes below them. Replaces the previous
// separate BepInEx and agent status cards. See docs/ARCHITECTURE.md §12.
import { Alert, Badge, Button, Group, Skeleton, Switch, Text } from '@mantine/core'
import { IconAlertTriangle, IconPlugConnected } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { openConfirmInstallAgent } from '../agent/openConfirmInstallAgent'
import { useAgent, useInstallAgent } from '../agent/useAgent'
import { useInstance } from '../instances/useInstance'
import { SectionCard, StatusPill } from '../../ui'
import { openConfirmInstallBepinex } from './openConfirmInstallBepinex'
import { useInstallBepinex, useModsOverview, useSetBepinexEnabled } from './useMods'
import classes from './ModLoaderCard.module.css'

export function ModLoaderCard({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const overview = useModsOverview(id)
  const instance = useInstance(id)
  const agent = useAgent(id)
  const installBepinex = useInstallBepinex(id)
  const setEnabled = useSetBepinexEnabled(id)
  const installAgent = useInstallAgent(id)

  const isRunning = ['running', 'starting', 'stopping'].includes(instance.data?.status.state ?? '')
  const canOperate = hasRole('operator')

  if (overview.isLoading || agent.isLoading) {
    return (
      <SectionCard title="Mod loader">
        <Skeleton height={88} />
      </SectionCard>
    )
  }

  const bepinex = overview.data?.bepinex
  const bepinexInstalled = !!overview.data?.bepinex.installed
  const info = agent.data

  let state: { color: string; label: string }
  if (!info?.installed) state = { color: 'gray', label: 'not installed' }
  else if (info.connected) state = { color: 'moss', label: `connected, v${info.installed_version ?? '?'}` }
  else if (!info.enabled) state = { color: 'gray', label: `installed v${info.installed_version ?? '?'}, BepInEx disabled` }
  else if (isRunning) state = { color: 'orange', label: `installed v${info.installed_version ?? '?'}, not reachable` }
  else state = { color: 'gray', label: `installed v${info.installed_version ?? '?'}, server stopped` }

  // Each condition matches the old BepInEx/agent status cards' bodies
  // verbatim; named here only so the wrapping .notes div can skip rendering
  // (and its padding) when none of them have anything to say.
  const pendingRestart = !!overview.data?.pending_restart
  const needsBepinexFirst = !bepinexInstalled
  const lastError = bepinexInstalled && !!info?.installed && !info.connected && !!info.last_error && isRunning
  const agentStatusLine = bepinexInstalled && !!info?.installed && info.connected && !!info.status
  const hasNotes = pendingRestart || needsBepinexFirst || lastError || agentStatusLine

  return (
    <SectionCard title="Mod loader" flush>
      <div className={classes.row}>
        <div>
          <Text size="sm" fw={600}>
            BepInEx
          </Text>
          <Text size="xs" c="dimmed" lineClamp={2}>
            The mod loader required to run mods on this server. Install it once, then browse the mod catalogue or upload mods below.
          </Text>
        </div>
        <Group gap="xs" wrap="nowrap">
          <StatusPill color={bepinex?.installed ? 'moss' : 'gray'}>
            {bepinex?.installed ? `installed ${bepinex.version ?? ''}`.trim() : 'not installed'}
          </StatusPill>
          {bepinex?.update_available && (
            <Badge color="frost" variant="light">
              latest {bepinex.latest_version}
            </Badge>
          )}
        </Group>
        {canOperate && bepinex?.installed ? (
          <Switch
            size="sm"
            checked={bepinex.enabled}
            label="Enabled"
            onChange={(e) => setEnabled.mutate(e.currentTarget.checked)}
            disabled={setEnabled.isPending}
          />
        ) : (
          <span />
        )}
        {canOperate && (
          <Button
            size="xs"
            variant={bepinex?.installed ? 'default' : 'filled'}
            loading={installBepinex.isPending}
            onClick={() =>
              openConfirmInstallBepinex({
                upgrade: !!bepinex?.installed,
                isRunning,
                onConfirm: (stop) => installBepinex.mutate({ stop_if_running: stop }),
              })
            }
          >
            {bepinex?.installed
              ? bepinex.update_available
                ? `Upgrade to ${bepinex.latest_version}`
                : 'Reinstall'
              : 'Install BepInEx'}
          </Button>
        )}
      </div>
      <div className={classes.row}>
        <div>
          <Text size="sm" fw={600}>
            Valheim UI Agent
          </Text>
          <Text size="xs" c="dimmed" lineClamp={2}>
            The manager's own server plugin: live players and world state on the Overview, kick and broadcast from the UI. Installed together with BepInEx; players need nothing.
          </Text>
        </div>
        <Group gap="xs" wrap="nowrap">
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
        </Group>
        <span />
        {hasRole('operator') && bepinexInstalled && (!info?.installed || info.update_available) && (
          <Button
            size="xs"
            variant="default"
            loading={installAgent.isPending}
            onClick={() =>
              openConfirmInstallAgent({
                update: !!info?.installed,
                isRunning,
                onConfirm: (stop) => installAgent.mutate({ stop_if_running: stop }),
              })
            }
          >
            {info?.installed ? 'Update agent' : 'Install agent'}
          </Button>
        )}
      </div>
      {hasNotes && (
        <div className={classes.notes}>
          {pendingRestart && (
            <Alert color="straw" icon={<IconAlertTriangle size={16} />} title="Restart required">
              Mod changes are staged. Restart the instance from the Overview tab to apply them.
            </Alert>
          )}
          {needsBepinexFirst && (
            <Text size="sm" c="dimmed">
              Install BepInEx above; the agent comes with it.
            </Text>
          )}
          {lastError && (
            <Text size="xs" c="dimmed">
              Last error: {info?.last_error}
            </Text>
          )}
          {agentStatusLine && info?.status && (
            <Text size="sm" c="dimmed">
              Agent v{info.status.agent_version} on game {info.status.game_version || '?'}, {info.status.players.length}{' '}
              player{info.status.players.length === 1 ? '' : 's'} online.
            </Text>
          )}
        </div>
      )}
    </SectionCard>
  )
}
