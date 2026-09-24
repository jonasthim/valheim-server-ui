// Manual mod upload: a .zip (Thunderstore layout with manifest.json) or a
// single .dll, dropped or picked, uploaded as multipart/form-data. A single
// 48px dropzone row under the installed mods table.
import { Group, Text, ThemeIcon } from '@mantine/core'
import { Dropzone } from '@mantine/dropzone'
import { IconFileZip, IconUpload, IconX } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { useJobDrawer } from '../jobs'
import { notifyError } from '../../lib/notify'
import { useUploadMod } from './useMods'
import classes from './UploadModCard.module.css'

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
    <Dropzone
      aria-label="Upload a mod"
      className={classes.dropRow}
      onDrop={handleDrop}
      onReject={() => notifyError(new Error('File was rejected.'), 'Upload failed')}
      loading={upload.isPending}
      multiple={false}
      maxSize={200 * 1024 * 1024}
    >
      <Group justify="space-between" wrap="nowrap" gap="sm" mih={48} px="sm" style={{ pointerEvents: 'none' }}>
        <Group gap="sm" wrap="nowrap">
          <Dropzone.Accept>
            <ThemeIcon size={24} color="moss" variant="light">
              <IconUpload size={16} />
            </ThemeIcon>
          </Dropzone.Accept>
          <Dropzone.Reject>
            <ThemeIcon size={24} color="blood" variant="light">
              <IconX size={16} />
            </ThemeIcon>
          </Dropzone.Reject>
          <Dropzone.Idle>
            <ThemeIcon size={24} color="spirit" variant="light">
              <IconFileZip size={16} />
            </ThemeIcon>
          </Dropzone.Idle>
          <Text size="sm">Drag a mod .zip or .dll here, or click to browse</Text>
        </Group>
        <Text size="xs" c="dimmed" visibleFrom="sm">
          Thunderstore-layout zips (with manifest.json) or a single BepInEx plugin .dll
        </Text>
      </Group>
    </Dropzone>
  )
}
