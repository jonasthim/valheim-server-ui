// BepInEx (mod loader) status card: install/upgrade, enable toggle, and the
// pending-restart hint. See docs/ARCHITECTURE.md §12.
import { Alert, Badge, Button, Card, Group, Skeleton, Stack, Switch, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconAlertTriangle, IconPuzzle } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
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
          <Text size="sm">
            BepInEx must be installed while the instance is <strong>stopped</strong>.
          </Text>
          {isRunning && (
            <Text size="sm" c="dimmed">
              This instance is currently running. Confirming will stop it, {verb.toLowerCase()} BepInEx, and start it
              again afterwards.
            </Text>
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
      <Card withBorder>
        <Skeleton height={80} />
      </Card>
    )
  }

  const bepinex = overview.data?.bepinex
  const canOperate = hasRole('operator')

  return (
    <Card withBorder>
      <Stack gap="sm">
        <Group justify="space-between" wrap="nowrap">
          <Group gap="xs">
            <IconPuzzle size={20} />
            <Text fw={600}>BepInEx</Text>
            {bepinex?.installed ? (
              <Badge color="green" variant="light">
                installed {bepinex.version ?? ''}
              </Badge>
            ) : (
              <Badge color="gray" variant="light">
                not installed
              </Badge>
            )}
            {bepinex?.installed && bepinex.latest_version && bepinex.latest_version !== bepinex.version && (
              <Badge color="blue" variant="light">
                latest {bepinex.latest_version}
              </Badge>
            )}
          </Group>
          {canOperate && (
            <Group gap="sm">
              {bepinex?.installed && (
                <Switch
                  checked={bepinex.enabled}
                  label="Enabled"
                  onChange={(e) => setEnabled.mutate(e.currentTarget.checked)}
                  disabled={setEnabled.isPending}
                />
              )}
              <Button
                size="xs"
                variant={bepinex?.installed ? 'light' : 'filled'}
                loading={installBepinex.isPending}
                onClick={() => confirmInstall(!!bepinex?.installed)}
              >
                {bepinex?.installed
                  ? bepinex.latest_version && bepinex.latest_version !== bepinex.version
                    ? `Upgrade to ${bepinex.latest_version}`
                    : 'Reinstall'
                  : 'Install BepInEx'}
              </Button>
            </Group>
          )}
        </Group>

        <Text size="sm" c="dimmed">
          BepInEx is the mod loader required to run Thunderstore mods on this server. Install it once, then browse
          Thunderstore or upload mods below.
        </Text>

        {overview.data?.pending_restart && (
          <Alert color="yellow" icon={<IconAlertTriangle size={16} />} title="Restart required">
            Mod changes are staged. Restart the instance from the Overview tab to apply them.
          </Alert>
        )}
      </Stack>
    </Card>
  )
}
