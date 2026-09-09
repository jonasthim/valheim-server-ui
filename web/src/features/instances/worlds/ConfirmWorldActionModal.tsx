import { useState } from 'react'
import { Alert, Button, Checkbox, Group, Stack, Text, TextInput } from '@mantine/core'
import { IconAlertTriangle } from '@tabler/icons-react'

/**
 * Destructive confirmation for world actions: the operator must type the world
 * name exactly before the button enables. Rendered through modals.open (see
 * openConfirmWorldAction) so it can hold local state.
 */
export function ConfirmWorldActionModal({
  worldName,
  action,
  instanceRunning,
  onConfirm,
  onCancel,
}: {
  worldName: string
  action: 'delete' | 'regenerate'
  instanceRunning: boolean
  onConfirm: (opts: { stopIfRunning: boolean }) => void
  onCancel: () => void
}) {
  const [typed, setTyped] = useState('')
  const [stopIfRunning, setStopIfRunning] = useState(true)
  const matches = typed === worldName
  const isRegenerate = action === 'regenerate'

  return (
    <Stack gap="sm">
      <Alert color="red" icon={<IconAlertTriangle size={18} />} title="This cannot be undone">
        {isRegenerate ? (
          <Text size="sm">
            The save files of <strong>{worldName}</strong> are deleted and Valheim generates a brand-new world with a{' '}
            <strong>new random seed</strong> and the same name on the next start. Every building, chest, portal and
            explored area is lost. Player characters are stored on the players' own machines and are not affected.
            A backup of the current world is taken first and kept until you delete it.
          </Text>
        ) : (
          <Text size="sm">
            The save files of <strong>{worldName}</strong> are deleted from this server, including Valheim's own
            rolling copies. Download it or take a backup first if you might want it back.
          </Text>
        )}
      </Alert>
      {isRegenerate && instanceRunning && (
        <Checkbox
          label="Stop the server, regenerate, and start it again"
          description="Players online are disconnected. Leave unchecked to be refused while the server runs."
          checked={stopIfRunning}
          onChange={(e) => setStopIfRunning(e.currentTarget.checked)}
        />
      )}
      <TextInput
        label={`Type ${worldName} to confirm`}
        placeholder={worldName}
        value={typed}
        onChange={(e) => setTyped(e.currentTarget.value)}
        autoComplete="off"
        data-autofocus
      />
      <Group justify="flex-end">
        <Button variant="default" onClick={onCancel}>
          Cancel
        </Button>
        <Button color="red" disabled={!matches} onClick={() => onConfirm({ stopIfRunning })}>
          {isRegenerate ? 'Regenerate world' : 'Delete world'}
        </Button>
      </Group>
    </Stack>
  )
}
