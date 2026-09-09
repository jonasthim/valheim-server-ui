import { useState } from 'react'
import { Button, Card, Group, TextInput, Title } from '@mantine/core'
import { IconDeviceFloppy } from '@tabler/icons-react'
import { useCreateBackup } from './useBackups'

export function BackupNowCard({ id }: { id: string }) {
  const [note, setNote] = useState('')
  const create = useCreateBackup(id)

  return (
    <Card withBorder>
      <Title order={4} mb="sm">
        Back up now
      </Title>
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
    </Card>
  )
}
