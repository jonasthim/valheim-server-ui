// Sends an on-screen message to every connected player through the agent.
import { useState } from 'react'
import { Button, Group, Modal, SegmentedControl, Stack, Text, TextInput } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconSpeakerphone } from '@tabler/icons-react'
import { useAgentCommand } from './useAgent'

export function BroadcastButton({ id, disabled }: { id: string; disabled?: boolean }) {
  const [opened, { open, close }] = useDisclosure(false)
  const [message, setMessage] = useState('')
  const [style, setStyle] = useState<'center' | 'topleft'>('center')
  const command = useAgentCommand(id)

  function send() {
    const text = message.trim()
    if (!text) return
    command.mutate(
      { command: 'broadcast', message: text, style },
      {
        onSuccess: (res) => {
          if (res.ok) {
            setMessage('')
            close()
          }
        },
      },
    )
  }

  return (
    <>
      <Button size="xs" variant="light" leftSection={<IconSpeakerphone size={14} />} onClick={open} disabled={disabled}>
        Broadcast
      </Button>
      <Modal opened={opened} onClose={close} title="Message all players" centered>
        <Stack gap="sm">
          <TextInput
            label="Message"
            placeholder="Server restarts in 5 minutes"
            maxLength={200}
            value={message}
            onChange={(e) => setMessage(e.currentTarget.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') send()
            }}
            data-autofocus
          />
          <SegmentedControl
            value={style}
            onChange={(v) => setStyle(v as 'center' | 'topleft')}
            data={[
              { value: 'center', label: 'Centre of screen' },
              { value: 'topleft', label: 'Top-left notice' },
            ]}
          />
          <Text size="xs" c="dimmed">
            Shown in-game the way raids and sleeping are announced. Up to 200 characters.
          </Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={close}>
              Cancel
            </Button>
            <Button onClick={send} loading={command.isPending} disabled={!message.trim()}>
              Send
            </Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}
