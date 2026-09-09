// Danger-zone confirm modal for ConfigTab (admin only): deletes an instance,
// optionally also removing its files on disk.
import { useState } from 'react'
import { Button, Checkbox, Group, Modal, Stack, Text } from '@mantine/core'
import { useDeleteInstance } from './instanceActions'

export function DeleteInstanceModal({
  id,
  name,
  opened,
  onClose,
  onDeleted,
}: {
  id: string
  name: string
  opened: boolean
  onClose: () => void
  onDeleted: () => void
}) {
  const [deleteFiles, setDeleteFiles] = useState(false)
  const deleteInstance = useDeleteInstance(id)

  function confirm() {
    deleteInstance.mutate(deleteFiles, {
      onSuccess: () => {
        onClose()
        onDeleted()
      },
    })
  }

  return (
    <Modal opened={opened} onClose={onClose} title="Delete instance" centered>
      <Stack gap="md">
        <Text size="sm">
          Delete <strong>{name}</strong> ({id})? This stops any running server and cannot be undone.
        </Text>
        <Checkbox
          label="Also delete files on disk (game files, saves, backups)"
          checked={deleteFiles}
          onChange={(e) => setDeleteFiles(e.currentTarget.checked)}
        />
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button color="red" loading={deleteInstance.isPending} onClick={confirm}>
            Delete instance
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
