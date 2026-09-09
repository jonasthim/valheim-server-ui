import { useState } from 'react'
import { Button, Group, TextInput } from '@mantine/core'
import { IconDeviceFloppy } from '@tabler/icons-react'
import { SectionCard } from '../../../ui'
import { useCreateBackup } from './useBackups'

export function BackupNowCard({ id }: { id: string }) {
  const [note, setNote] = useState('')
  const create = useCreateBackup(id)

  return (
    <SectionCard title="Back up now" description="Create a manual backup of the current world and config.">
      <Group align="flex-end" wrap="wrap">
        <TextInput
          label="Note (optional)"
          placeholder="e.g. before the raid"
          value={note}
          onChange={(e) => setNote(e.currentTarget.value)}
          maxLength={200}
          style={{ flex: 1, minWidth: 220 }}
        />
        <Button
          leftSection={<IconDeviceFloppy size={16} />}
          loading={create.isPending}
          onClick={() => create.mutate(note, { onSuccess: () => setNote('') })}
        >
          Back up now
        </Button>
      </Group>
    </SectionCard>
  )
}
