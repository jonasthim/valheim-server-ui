// Lets an operator merge a player's character file (.fch) into the fog of
// war: the file holds the map that character has explored in every world.
import { useState } from 'react'
import { Button, Code, FileInput, Group, Modal, Stack, Text } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconFileImport } from '@tabler/icons-react'
import { useImportExplored } from './useMap'

export function ImportExploredButton({ id, disabled }: { id: string; disabled?: boolean }) {
  const [opened, { open, close }] = useDisclosure(false)
  const [file, setFile] = useState<File | null>(null)
  const importExplored = useImportExplored(id)

  function submit() {
    if (!file) return
    importExplored.mutate(file, {
      onSuccess: (res) => {
        if (res.ok) {
          setFile(null)
          close()
        }
      },
    })
  }

  return (
    <>
      <Button size="xs" variant="light" leftSection={<IconFileImport size={14} />} onClick={open} disabled={disabled}>
        Import character map
      </Button>
      <Modal opened={opened} onClose={close} title="Import a character's explored map" centered size="lg">
        <Stack gap="sm">
          <Text size="sm">
            Valheim keeps each player's exploration in their character file, not on the server. Upload a character
            file and everything that character has explored in this world is added to the fog of war. Repeat for
            every player who wants their discoveries on the map.
          </Text>
          <Text size="sm" c="dimmed">
            Where the file lives (the player copies it while the game is closed):
          </Text>
          <Stack gap={4}>
            <Text size="xs">
              Windows: <Code>%USERPROFILE%\AppData\LocalLow\IronGate\Valheim\characters_local\&lt;name&gt;.fch</Code>
            </Text>
            <Text size="xs">
              Linux: <Code>~/.config/unity3d/IronGate/Valheim/characters_local/&lt;name&gt;.fch</Code>
            </Text>
            <Text size="xs">
              Steam Cloud characters: <Code>Steam\userdata\&lt;id&gt;\892970\remote\characters\</Code>
            </Text>
          </Stack>
          <FileInput
            label="Character file"
            placeholder="Pick a .fch file"
            accept=".fch,.old"
            value={file}
            onChange={setFile}
            clearable
          />
          <Text size="xs" c="dimmed">
            Only the map data for this world is read; the file itself is not stored. The server must be running.
          </Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={close}>
              Cancel
            </Button>
            <Button onClick={submit} loading={importExplored.isPending} disabled={!file}>
              Import
            </Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}
