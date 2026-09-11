// Restart button with a delay menu. "Restart now" behaves as before (a
// confirm when players are online); the timed options enqueue a graceful
// restart that warns connected players with an in-game countdown before the
// server goes down. The delay is chosen here (presets or a custom value).
import { useState } from 'react'
import { Button, Group, Menu, Modal, NumberInput, Stack, Text } from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconChevronDown, IconRefresh } from '@tabler/icons-react'
import { useJobDrawer } from '../jobs'
import { useRestartInstance } from './instanceActions'

const PRESETS: { label: string; seconds: number }[] = [
  { label: 'In 1 minute', seconds: 60 },
  { label: 'In 2 minutes', seconds: 120 },
  { label: 'In 5 minutes', seconds: 300 },
  { label: 'In 10 minutes', seconds: 600 },
]

export function RestartControl({
  id,
  disabled,
  playersOnline,
}: {
  id: string
  disabled?: boolean
  playersOnline: number
}) {
  const restart = useRestartInstance(id)
  const { openJob } = useJobDrawer()
  const [customOpen, setCustomOpen] = useState(false)
  const [minutes, setMinutes] = useState<number | string>(5)

  function restartIn(seconds: number) {
    restart.mutate(seconds, { onSuccess: (res) => res.job && openJob(res.job.id) })
  }

  function restartNow() {
    if (playersOnline > 0) {
      modals.openConfirmModal({
        title: 'Restart instance',
        children: (
          <Text size="sm">
            {playersOnline} player{playersOnline === 1 ? ' is' : 's are'} currently online and will be disconnected
            immediately. Restart now, or pick a delay to warn them first.
          </Text>
        ),
        labels: { confirm: 'Restart now', cancel: 'Cancel' },
        confirmProps: { color: 'orange' },
        onConfirm: () => restartIn(0),
      })
    } else {
      restartIn(0)
    }
  }

  return (
    <>
      <Menu shadow="md" position="bottom-end" withinPortal>
        <Menu.Target>
          <Button
            size="xs"
            variant="outline"
            leftSection={<IconRefresh size={14} />}
            rightSection={<IconChevronDown size={14} />}
            disabled={disabled}
            loading={restart.isPending}
          >
            Restart
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Item onClick={restartNow}>Restart now</Menu.Item>
          <Menu.Divider />
          <Menu.Label>Warn players, then restart</Menu.Label>
          {PRESETS.map((p) => (
            <Menu.Item key={p.seconds} onClick={() => restartIn(p.seconds)}>
              {p.label}
            </Menu.Item>
          ))}
          <Menu.Item onClick={() => setCustomOpen(true)}>Custom…</Menu.Item>
        </Menu.Dropdown>
      </Menu>

      <Modal opened={customOpen} onClose={() => setCustomOpen(false)} title="Restart with a warning" centered>
        <Stack gap="md">
          <NumberInput
            label="Warn players, then restart after (minutes)"
            min={1}
            max={60}
            value={minutes}
            onChange={(v) => setMinutes(typeof v === 'bigint' ? Number(v) : v)}
            data-autofocus
          />
          <Text size="xs" c="dimmed">
            Players get an in-game countdown. An empty server restarts right away.
          </Text>
          <Group justify="flex-end">
            <Button variant="default" onClick={() => setCustomOpen(false)}>
              Cancel
            </Button>
            <Button
              onClick={() => {
                const secs = Math.max(1, Number(minutes) || 0) * 60
                setCustomOpen(false)
                restartIn(secs)
              }}
            >
              Schedule restart
            </Button>
          </Group>
        </Stack>
      </Modal>
    </>
  )
}
