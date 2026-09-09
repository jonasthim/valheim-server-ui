// "What's new" modal for the manager's own release, opened from the
// dashboard banner and the settings Application card.
import { Anchor, Modal, ScrollArea, Stack, Text } from '@mantine/core'
import type { AppUpdateInfo } from '../../api/types'
import { fmtAgo, safeHref } from '../../lib/format'

export function ReleaseNotesModal({
  opened,
  onClose,
  appUpdate,
}: {
  opened: boolean
  onClose: () => void
  appUpdate: AppUpdateInfo
}) {
  return (
    <Modal
      opened={opened}
      onClose={onClose}
      title={`Valheim Server UI ${appUpdate.latest_version ?? ''}`}
      size="lg"
      radius="lg"
    >
      <Stack gap="sm">
        {appUpdate.published_at && (
          <Text size="sm" c="dimmed">
            Published {fmtAgo(appUpdate.published_at)}
          </Text>
        )}
        <ScrollArea.Autosize mah={400}>
          <Text component="pre" size="sm" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
            {appUpdate.release_notes || 'No release notes provided.'}
          </Text>
        </ScrollArea.Autosize>
        {appUpdate.release_url && (
          <Anchor href={safeHref(appUpdate.release_url)} target="_blank" rel="noreferrer" size="sm">
            View release on GitHub
          </Anchor>
        )}
      </Stack>
    </Modal>
  )
}
