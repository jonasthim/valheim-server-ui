// Manual mod upload: a .zip (Thunderstore layout with manifest.json) or a
// single .dll, dropped or picked, uploaded as multipart/form-data.
import { Group, Text, ThemeIcon } from '@mantine/core'
import { Dropzone } from '@mantine/dropzone'
import { IconFileZip, IconUpload, IconX } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { useJobDrawer } from '../jobs'
import { notifyError } from '../../lib/notify'
import { SectionCard } from '../../ui'
import { useUploadMod } from './useMods'

const ACCEPTED_EXT = ['.zip', '.dll']

export function UploadModCard({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const { openJob } = useJobDrawer()
  const upload = useUploadMod(id)

  if (!hasRole('operator')) return null

  function handleDrop(files: File[]) {
    const file = files[0]
    if (!file) return
    const lower = file.name.toLowerCase()
    if (!ACCEPTED_EXT.some((ext) => lower.endsWith(ext))) {
      notifyError(new Error('Only .zip or .dll files are accepted.'), 'Unsupported file')
      return
    }
    upload.mutate(file, { onSuccess: (res) => openJob(res.job.id) })
  }

  return (
    <SectionCard title="Upload a mod">
      <Dropzone
        onDrop={handleDrop}
        onReject={() => notifyError(new Error('File was rejected.'), 'Upload failed')}
        loading={upload.isPending}
        multiple={false}
        maxSize={200 * 1024 * 1024}
        style={{ background: 'var(--vh-surface-2)', borderColor: 'var(--vh-border-strong)' }}
      >
        <Group justify="center" gap="md" mih={100} style={{ pointerEvents: 'none' }}>
          <Dropzone.Accept>
            <ThemeIcon size={40} color="moss" variant="light">
              <IconUpload size={22} />
            </ThemeIcon>
          </Dropzone.Accept>
          <Dropzone.Reject>
            <ThemeIcon size={40} color="blood" variant="light">
              <IconX size={22} />
            </ThemeIcon>
          </Dropzone.Reject>
          <Dropzone.Idle>
            <ThemeIcon size={40} color="spirit" variant="light">
              <IconFileZip size={22} />
            </ThemeIcon>
          </Dropzone.Idle>
          <div>
            <Text size="sm">Drag a mod .zip or .dll here, or click to browse</Text>
            <Text size="xs" c="dimmed">
              Thunderstore-layout zips (with manifest.json) or a single BepInEx plugin .dll
            </Text>
          </div>
        </Group>
      </Dropzone>
    </SectionCard>
  )
}
