// Operator controls for the running world, driven by the agent's 1.11 command
// contract: set the time of day, start or stop a random event, and manage the
// world's global (progression) keys. The event and key pickers come from
// GET /agent/catalog. Kept off the card header behind one "World controls"
// menu so Save and Broadcast stay the primary actions.
import { useState } from 'react'
import {
  Alert,
  Autocomplete,
  Badge,
  Button,
  Group,
  Menu,
  Modal,
  NumberInput,
  SegmentedControl,
  Select,
  Slider,
  Stack,
  Text,
} from '@mantine/core'
import {
  IconAdjustments,
  IconClockHour4,
  IconKey,
  IconSwords,
} from '@tabler/icons-react'
import type { AgentStatus } from '../../api/types'
import { fmtWorldTime, useAgentCatalog, useAgentCommand } from './useAgent'

type Panel = 'time' | 'event' | 'keys' | null

// Mirrors the plugin's global-key rule so the Set button gates bad input.
const KEY_PATTERN = /^[A-Za-z0-9_]{1,64}$/

export function WorldControls({
  id,
  status,
  disabled,
}: {
  id: string
  status: AgentStatus | undefined
  disabled?: boolean
}) {
  const [panel, setPanel] = useState<Panel>(null)
  // Load the pickers only once a panel that needs them is opened.
  const catalog = useAgentCatalog(id, panel === 'event' || panel === 'keys')

  return (
    <>
      <Menu shadow="md" position="bottom-end" withinPortal>
        <Menu.Target>
          <Button size="xs" variant="light" leftSection={<IconAdjustments size={14} />} disabled={disabled}>
            World controls
          </Button>
        </Menu.Target>
        <Menu.Dropdown>
          <Menu.Item leftSection={<IconClockHour4 size={14} />} onClick={() => setPanel('time')}>
            Set time of day
          </Menu.Item>
          <Menu.Item leftSection={<IconSwords size={14} />} onClick={() => setPanel('event')}>
            Start an event
          </Menu.Item>
          <Menu.Item leftSection={<IconKey size={14} />} onClick={() => setPanel('keys')}>
            Manage global keys
          </Menu.Item>
        </Menu.Dropdown>
      </Menu>

      <TimeModal id={id} opened={panel === 'time'} onClose={() => setPanel(null)} />
      <EventModal
        id={id}
        opened={panel === 'event'}
        onClose={() => setPanel(null)}
        events={catalog.data?.events ?? []}
        loading={catalog.isLoading}
        running={status?.world?.event?.name}
      />
      <KeysModal
        id={id}
        opened={panel === 'keys'}
        onClose={() => setPanel(null)}
        current={status?.global_keys ?? []}
        known={catalog.data?.global_keys ?? []}
        loading={catalog.isLoading}
      />
    </>
  )
}

function TimeModal({ id, opened, onClose }: { id: string; opened: boolean; onClose: () => void }) {
  const command = useAgentCommand(id)
  const [mode, setMode] = useState<'clock' | 'morning' | 'advance'>('clock')
  const [fraction, setFraction] = useState(0.5)
  const [minutes, setMinutes] = useState<number | string>(60)

  function apply() {
    const done = { onSuccess: (res: { ok: boolean }) => res.ok && onClose() }
    if (mode === 'morning') command.mutate({ command: 'time', skip: 'morning' }, done)
    else if (mode === 'clock') command.mutate({ command: 'time', fraction }, done)
    else command.mutate({ command: 'time', seconds: Math.max(1, Number(minutes) || 0) * 60 }, done)
  }

  return (
    <Modal opened={opened} onClose={onClose} title="Set the world time" centered>
      <Stack gap="md">
        <SegmentedControl
          fullWidth
          value={mode}
          onChange={(v) => setMode(v as typeof mode)}
          data={[
            { value: 'clock', label: 'Time of day' },
            { value: 'morning', label: 'Skip to morning' },
            { value: 'advance', label: 'Advance' },
          ]}
        />
        {mode === 'clock' && (
          <div>
            <Group justify="space-between" mb={4}>
              <Text size="sm">Set the clock to</Text>
              <Badge variant="light">{fmtWorldTime(fraction)}</Badge>
            </Group>
            <Slider
              min={0}
              max={1}
              step={1 / 48}
              value={fraction}
              onChange={setFraction}
              label={(v) => fmtWorldTime(v)}
              marks={[
                { value: 0, label: '00:00' },
                { value: 0.25, label: '06:00' },
                { value: 0.5, label: '12:00' },
                { value: 0.75, label: '18:00' },
              ]}
            />
            <Text size="xs" c="dimmed" mt="lg">
              The clock only moves forward, so a time earlier than now lands tomorrow.
            </Text>
          </div>
        )}
        {mode === 'morning' && (
          <Text size="sm" c="dimmed">
            Skips to the next morning, exactly as sleeping does. Every client follows the server clock.
          </Text>
        )}
        {mode === 'advance' && (
          <NumberInput
            label="Advance by (minutes)"
            min={1}
            max={1440}
            value={minutes}
            onChange={(v) => setMinutes(typeof v === 'bigint' ? Number(v) : v)}
            description="Moves the clock forward from now."
          />
        )}
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={apply} loading={command.isPending}>
            Apply
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

function EventModal({
  id,
  opened,
  onClose,
  events,
  loading,
  running,
}: {
  id: string
  opened: boolean
  onClose: () => void
  events: { name: string; duration_seconds: number }[]
  loading: boolean
  running?: string
}) {
  const command = useAgentCommand(id)
  const [name, setName] = useState<string | null>(null)

  function start() {
    if (!name) return
    command.mutate({ command: 'event', event: name }, { onSuccess: (res) => res.ok && onClose() })
  }
  function stop() {
    command.mutate({ command: 'eventstop' }, { onSuccess: (res) => res.ok && onClose() })
  }

  const data = events.map((e) => ({
    value: e.name,
    label: `${e.name} (${Math.round(e.duration_seconds / 60)} min)`,
  }))

  return (
    <Modal opened={opened} onClose={onClose} title="Random events" centered>
      <Stack gap="md">
        {running && (
          <Alert color="straw" title={`"${running}" is running`}>
            <Button size="xs" color="red" variant="light" onClick={stop} loading={command.isPending && command.variables?.command === 'eventstop'}>
              Stop the event
            </Button>
          </Alert>
        )}
        <Select
          label="Event"
          placeholder={loading ? 'Loading events…' : 'Pick an event'}
          data={data}
          value={name}
          onChange={(v) => setName(v)}
          searchable
          nothingFoundMessage="No events"
          disabled={loading}
        />
        <Text size="xs" c="dimmed">
          Events spawn around players. With nobody online, the event may not fire until someone connects.
        </Text>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Cancel
          </Button>
          <Button onClick={start} loading={command.isPending && command.variables?.command === 'event'} disabled={!name}>
            Start event
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}

function KeysModal({
  id,
  opened,
  onClose,
  current,
  known,
  loading,
}: {
  id: string
  opened: boolean
  onClose: () => void
  current: string[]
  known: string[]
  loading: boolean
}) {
  const command = useAgentCommand(id)
  const [toAdd, setToAdd] = useState<string | null>(null)

  function setKey(key: string) {
    command.mutate({ command: 'setkey', key }, { onSuccess: () => setToAdd(null) })
  }
  function removeKey(key: string) {
    command.mutate({ command: 'removekey', key })
  }

  // Offer known keys the world does not already have, plus free entry.
  const options = known.filter((k) => !current.includes(k)).map((k) => ({ value: k, label: k }))

  return (
    <Modal opened={opened} onClose={onClose} title="Global keys" centered>
      <Stack gap="md">
        <Text size="sm" c="dimmed">
          Global keys are progression flags such as boss defeats. Setting one unlocks what it gates for everyone.
        </Text>
        <Group align="flex-end" gap="sm" wrap="nowrap">
          <Autocomplete
            label="Add a key"
            placeholder={loading ? 'Loading…' : 'Pick or type a key'}
            data={options.map((o) => o.value)}
            value={toAdd ?? ''}
            onChange={setToAdd}
            style={{ flex: 1 }}
            disabled={loading}
            error={toAdd && !KEY_PATTERN.test(toAdd) ? 'Letters, digits and _ only' : undefined}
          />
          <Button
            onClick={() => toAdd && setKey(toAdd)}
            loading={command.isPending && command.variables?.command === 'setkey'}
            disabled={!toAdd || !KEY_PATTERN.test(toAdd)}
          >
            Set
          </Button>
        </Group>
        <div>
          <Text size="xs" c="dimmed" mb={4}>
            Current keys ({current.length})
          </Text>
          <Group gap={6}>
            {current.length === 0 && (
              <Text size="sm" c="dimmed">
                none yet
              </Text>
            )}
            {current.map((k) => (
              <Badge
                key={k}
                variant="light"
                color="straw"
                size="sm"
                rightSection={
                  <Text
                    component="span"
                    size="xs"
                    style={{ cursor: 'pointer' }}
                    onClick={() => removeKey(k)}
                    aria-label={`Remove ${k}`}
                  >
                    ✕
                  </Text>
                }
              >
                {k}
              </Badge>
            ))}
          </Group>
        </div>
        <Group justify="flex-end">
          <Button variant="default" onClick={onClose}>
            Done
          </Button>
        </Group>
      </Stack>
    </Modal>
  )
}
