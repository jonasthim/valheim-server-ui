import { useState } from 'react'
import { Button, Card, Group, Text, Title } from '@mantine/core'
import { Dropzone } from '@mantine/dropzone'
import { IconUpload, IconX } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { useUploadBackup } from './useBackups'

export function BackupUploadCard({ id }: { id: string }) {
  const [file, setFile] = useState<File | null>(null)
  const upload = useUploadBackup(id)

  function handleDrop(files: File[]) {
    const zip = files.find((f) => f.name.toLowerCase().endsWith('.zip'))
    if (!zip) {
      notifications.show({ title: 'Not a backup', message: 'Only .zip files exported from this UI are accepted.', color: 'yellow' })
      return
    }
    setFile(zip)
  }

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        Upload a backup
      </Title>
      <Group align="center" wrap="wrap">
        <Dropzone onDrop={handleDrop} multiple={false} style={{ flex: 1, minWidth: 260 }}>
          <Group justify="center" gap="md" mih={70} style={{ pointerEvents: 'none' }}>
            <Dropzone.Accept>
              <IconUpload size={28} />
            </Dropzone.Accept>
            <Dropzone.Reject>
              <IconX size={28} />
            </Dropzone.Reject>
            <Dropzone.Idle>
              <IconUpload size={28} />
            </Dropzone.Idle>
            <Text size="sm">{file ? file.name : 'Drag a backup .zip previously downloaded from this UI'}</Text>
          </Group>
        </Dropzone>
        <Button
          disabled={!file}
          loading={upload.isPending}
          onClick={() => file && upload.mutate(file, { onSuccess: () => setFile(null) })}
        >
          Upload
        </Button>
      </Group>
    </Card>
  )
}
