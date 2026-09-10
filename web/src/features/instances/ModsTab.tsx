// Mods tab: BepInEx loader status, installed mods, manual upload, the
// Thunderstore browser modal, and the BepInEx config editor. Owned by WP-13
// (docs/WORKPLAN.md); kept thin, composing web/src/features/mods/*.
import { Button, Group, Stack, Text } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconWorldSearch } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import {
  BepInExCard,
  CheckModUpdatesButton,
  ConfigEditor,
  InstalledModsTable,
  ThunderstoreBrowser,
  UploadModCard,
  useModsOverview,
} from '../mods'
import { SectionCard } from '../../ui'
import { AgentCard } from '../agent'

export function ModsTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const overview = useModsOverview(id)
  const [browserOpen, browserHandlers] = useDisclosure(false)

  return (
    <Stack gap="lg">
      <BepInExCard id={id} />
      <AgentCard id={id} />

      {hasRole('operator') && (
        <SectionCard
          title="Browse mods"
          description="Search the Valheim mod catalogue and install mods with their dependencies."
          actions={
            <Group gap="xs">
              <CheckModUpdatesButton id={id} />
              <Button leftSection={<IconWorldSearch size={16} />} onClick={browserHandlers.open}>
                Browse mods
              </Button>
            </Group>
          }
        >
          <Text size="sm" c="dimmed">
            Installed mods appear below; updates are checked against the cached index.
          </Text>
        </SectionCard>
      )}

      <InstalledModsTable id={id} />

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
