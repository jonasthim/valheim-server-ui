import { useState } from 'react'
import { Button, Checkbox, Group, List, Stack, Text } from '@mantine/core'
import { Dropzone } from '@mantine/dropzone'
import { IconUpload, IconX } from '@tabler/icons-react'
import { notifications } from '@mantine/notifications'
import { SectionCard } from '../../../ui'
import { useUploadWorlds } from './useWorlds'

const ALLOWED_EXT = ['.db', '.fwl', '.zip']

function isAllowed(file: File): boolean {
  const lower = file.name.toLowerCase()
  return ALLOWED_EXT.some((ext) => lower.endsWith(ext))
}

export function WorldUploadCard({ id }: { id: string }) {
  const [files, setFiles] = useState<File[]>([])
  const [overwrite, setOverwrite] = useState(false)
  const upload = useUploadWorlds(id)

  function handleDrop(dropped: File[]) {
    const accepted = dropped.filter(isAllowed)
    const rejected = dropped.length - accepted.length
    if (rejected > 0) {
      notifications.show({
        title: 'Some files skipped',
        message: `Only .db, .fwl or .zip files are accepted (${rejected} file${rejected === 1 ? '' : 's'} ignored).`,
        color: 'yellow',
      })
    }
    setFiles((prev) => [...prev, ...accepted])
  }

  function submit() {
    upload.mutate(
      { files, overwrite },
      {
        onSuccess: () => {
          setFiles([])
          setOverwrite(false)
        },
      },
    )
  }

  return (
    <SectionCard title="Upload a world">
      <Stack gap="sm">
        <Dropzone onDrop={handleDrop} multiple>
          <Group justify="center" gap="md" mih={100} style={{ pointerEvents: 'none' }}>
            <Dropzone.Accept>
              <IconUpload size={32} />
            </Dropzone.Accept>
            <Dropzone.Reject>
              <IconX size={32} />
            </Dropzone.Reject>
            <Dropzone.Idle>
              <IconUpload size={32} />
            </Dropzone.Idle>
            <div>
              <Text size="sm" inline>
                Drag a <code>.db</code> + <code>.fwl</code> pair, or a single <code>.zip</code> containing them
              </Text>
              <Text size="xs" c="dimmed" inline mt={4}>
                Click to browse files
              </Text>
            </div>
          </Group>
        </Dropzone>

        {files.length > 0 && (
          <List size="sm" spacing={2}>
            {files.map((f, i) => (
              <List.Item key={`${f.name}-${i}`}>{f.name}</List.Item>
            ))}
          </List>
        )}

        <Checkbox
          label="Overwrite existing world files with the same name"
          checked={overwrite}
          onChange={(e) => setOverwrite(e.currentTarget.checked)}
        />

        <Group justify="flex-end">
          {files.length > 0 && (
            <Button variant="default" onClick={() => setFiles([])} disabled={upload.isPending}>
              Clear
            </Button>
          )}
          <Button onClick={submit} disabled={files.length === 0} loading={upload.isPending}>
            Upload
          </Button>
        </Group>
      </Stack>
    </SectionCard>
  )
}
