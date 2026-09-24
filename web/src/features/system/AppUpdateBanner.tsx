// Dashboard-facing "a new manager version is available" banner. Renders
// nothing when there is no update. Upgrade confirm/mutate/watch state lives
// in useUpgradeAppAction + UpgradeFlowHost, shared with SettingsPage.
import { useState } from 'react'
import { Alert, Button, Group, Text, Tooltip } from '@mantine/core'
import { IconRocket, IconSparkles } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import type { AppUpdateInfo } from '../../api/types'
import { fmtAgo } from '../../lib/format'
import { ReleaseNotesModal } from './ReleaseNotesModal'
import { useUpgradeAppAction } from './useUpgradeAppAction'

export function AppUpdateBanner({ appUpdate }: { appUpdate: AppUpdateInfo | undefined }) {
  const { hasRole } = useAuth()
  const [notesOpen, setNotesOpen] = useState(false)
  const { upgrade, isPending, inFlight, target } = useUpgradeAppAction()

  if (!appUpdate?.update_available) return null

  return (
    <>
      <Alert
        color="ember"
        variant="light"
        radius="lg"
        icon={<IconSparkles size={16} />}
        title={`Valheim Server UI ${appUpdate.latest_version ?? '?'} available`}
      >
        <Group justify="space-between" align="center" wrap="wrap">
          <Text size="sm" c="dimmed">
            Currently running v{appUpdate.current_version}
            {appUpdate.checked_at ? `, checked ${fmtAgo(appUpdate.checked_at)}` : ''}
          </Text>
          <Group gap="xs">
            <Button size="xs" variant="default" onClick={() => setNotesOpen(true)}>
              What's new
            </Button>
            {hasRole('admin') && (
              <Tooltip
                label={appUpdate.reason ?? 'Self-upgrade unavailable'}
                disabled={appUpdate.can_self_upgrade}
              >
                <Button
                  size="xs"
                  leftSection={<IconRocket size={14} />}
                  disabled={!appUpdate.can_self_upgrade || inFlight}
                  loading={isPending || inFlight}
                  onClick={upgrade}
                >
                  {inFlight ? `Upgrading${target ? ` to ${target}` : ''}…` : 'Upgrade'}
                </Button>
              </Tooltip>
            )}
          </Group>
        </Group>
      </Alert>

      <ReleaseNotesModal opened={notesOpen} onClose={() => setNotesOpen(false)} appUpdate={appUpdate} />
    </>
  )
}
