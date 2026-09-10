// BepInEx (mod loader) status card: install/upgrade, enable toggle, and the
// pending-restart hint. See docs/ARCHITECTURE.md §12.
import { Alert, Badge, Button, Group, Skeleton, Stack, Switch } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconAlertTriangle } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { SectionCard, StatusPill } from '../../ui'
import { useInstance } from '../instances/useInstance'
import { useInstallBepinex, useModsOverview, useSetBepinexEnabled } from './useMods'

export function BepInExCard({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const overview = useModsOverview(id)
  const instance = useInstance(id)
  const installBepinex = useInstallBepinex(id)
  const setEnabled = useSetBepinexEnabled(id)

  const isRunning = ['running', 'starting', 'stopping'].includes(instance.data?.status.state ?? '')

  function confirmInstall(upgrade: boolean) {
    const verb = upgrade ? 'Upgrade' : 'Install'
    modals.openConfirmModal({
      title: `${verb} BepInEx`,
      children: (
        <Stack gap="xs">
          <span>
            BepInEx must be installed while the instance is <strong>stopped</strong>.
          </span>
          {isRunning && (
            <span style={{ color: 'var(--vh-text-soft)' }}>
              This instance is currently running. Confirming will stop it, {verb.toLowerCase()} BepInEx, and start it
              again afterwards.
            </span>
          )}
        </Stack>
      ),
      labels: {
        confirm: isRunning ? `Stop, ${verb.toLowerCase()} and start again` : verb,
        cancel: 'Cancel',
      },
      onConfirm: () => installBepinex.mutate({ stop_if_running: isRunning }),
    })
  }

  if (overview.isLoading) {
    return (
      <SectionCard title="BepInEx">
        <Skeleton height={60} />
      </SectionCard>
    )
  }

  const bepinex = overview.data?.bepinex
  const canOperate = hasRole('operator')

  return (
    <SectionCard
      title="BepInEx"
      description="The mod loader required to run mods on this server. Install it once, then browse the mod catalogue or upload mods below."
      actions={
        <Group gap="sm" wrap="wrap" justify="flex-end">
          <StatusPill color={bepinex?.installed ? 'moss' : 'gray'}>
            {bepinex?.installed ? `installed ${bepinex.version ?? ''}`.trim() : 'not installed'}
          </StatusPill>
          {bepinex?.update_available && (
            <Badge color="frost" variant="light">
              latest {bepinex.latest_version}
            </Badge>
          )}
          {canOperate && bepinex?.installed && (
            <Switch
              checked={bepinex.enabled}
              label="Enabled"
              onChange={(e) => setEnabled.mutate(e.currentTarget.checked)}
              disabled={setEnabled.isPending}
            />
          )}
          {canOperate && (
            <Button
              size="xs"
              variant={bepinex?.installed ? 'light' : 'filled'}
              loading={installBepinex.isPending}
              onClick={() => confirmInstall(!!bepinex?.installed)}
            >
              {bepinex?.installed
                ? bepinex.update_available
                  ? `Upgrade to ${bepinex.latest_version}`
                  : 'Reinstall'
                : 'Install BepInEx'}
            </Button>
          )}
        </Group>
      }
    >
      {overview.data?.pending_restart && (
        <Alert color="straw" icon={<IconAlertTriangle size={16} />} title="Restart required">
          Mod changes are staged. Restart the instance from the Overview tab to apply them.
        </Alert>
      )}
    </SectionCard>
  )
}
