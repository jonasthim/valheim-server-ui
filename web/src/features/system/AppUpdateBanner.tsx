// Dashboard-facing "a new manager version is available" banner. Renders
// nothing when there is no update, but still keeps the restart watcher
// mounted so it can catch a self-upgrade job triggered from here.
import { useState } from 'react'
import { Alert, Button, Group, Text, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconRocket, IconSparkles } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import type { AppUpdateInfo } from '../../api/types'
import { fmtAgo } from '../../lib/format'
import { useJobDrawer } from '../jobs'
import { ManagerRestartOverlay } from './ManagerRestartOverlay'
import { ReleaseNotesModal } from './ReleaseNotesModal'
import { UPGRADE_EXPLANATION, useManagerRestartWatch, useUpgradeApp } from './useAppUpdate'

export function AppUpdateBanner({ appUpdate }: { appUpdate: AppUpdateInfo | undefined }) {
  const { hasRole } = useAuth()
  const { openJob } = useJobDrawer()
  const upgrade = useUpgradeApp()
  const [notesOpen, setNotesOpen] = useState(false)
  const [jobId, setJobId] = useState<string | undefined>(undefined)
  const { restarting } = useManagerRestartWatch(jobId)

  if (!appUpdate?.update_available) return null

  function confirmUpgrade() {
    modals.openConfirmModal({
      title: 'Upgrade Valheim Server UI',
      children: <Text size="sm">{UPGRADE_EXPLANATION}</Text>,
      labels: { confirm: 'Upgrade now', cancel: 'Cancel' },
      onConfirm: () =>
        upgrade.mutate(undefined, {
          onSuccess: (res) => {
            setJobId(res.job.id)
            openJob(res.job.id)
          },
        }),
    })
  }

  return (
    <>
      <Alert
        color="blue"
        variant="light"
        icon={<IconSparkles size={16} />}
        title={`Valheim Server UI ${appUpdate.latest_version ?? '?'} available`}
      >
        <Group justify="space-between" wrap="wrap" gap="sm">
          <Text size="sm" c="dimmed">
            Currently running v{appUpdate.current_version}
            {appUpdate.checked_at ? ` · checked ${fmtAgo(appUpdate.checked_at)}` : ''}
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
                  disabled={!appUpdate.can_self_upgrade}
                  loading={upgrade.isPending}
                  onClick={confirmUpgrade}
                >
                  Upgrade
                </Button>
              </Tooltip>
            )}
          </Group>
        </Group>
      </Alert>

      <ReleaseNotesModal opened={notesOpen} onClose={() => setNotesOpen(false)} appUpdate={appUpdate} />
      <ManagerRestartOverlay visible={restarting} />
    </>
  )
}
