// Mods tab: mod loader status, installed mods, manual upload, the
// Thunderstore browser modal, and the BepInEx config editor. Owned by WP-13
// (docs/WORKPLAN.md); kept thin, composing web/src/features/mods/*.
import { Button, Stack } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconWorldSearch } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import {
  CheckModUpdatesButton,
  ConfigEditor,
  InstalledModsTable,
  ModLoaderCard,
  ThunderstoreBrowser,
  UploadModCard,
  useModsOverview,
} from '../mods'
import { LoadError } from '../../ui'

export function ModsTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const overview = useModsOverview(id)
  const [browserOpen, browserHandlers] = useDisclosure(false)

  if (overview.isError) {
    return <LoadError error={overview.error} title="Could not load mods" onRetry={() => overview.refetch()} />
  }

  return (
    <Stack gap="md">
      <ModLoaderCard id={id} />

      <InstalledModsTable
        id={id}
        actions={
          hasRole('operator') && (
            <>
              <CheckModUpdatesButton id={id} size="xs" />
              <Button size="xs" leftSection={<IconWorldSearch size={14} />} onClick={browserHandlers.open}>
                Browse mods
              </Button>
            </>
          )
        }
      />

      {hasRole('operator') && <UploadModCard id={id} />}

      <ConfigEditor id={id} />

      <ThunderstoreBrowser
        id={id}
        opened={browserOpen}
        onClose={browserHandlers.close}
        installedMods={overview.data?.mods ?? []}
      />
    </Stack>
  )
}
