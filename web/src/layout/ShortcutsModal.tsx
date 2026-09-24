// `?` (also reachable from the avatar menu and the palette's Help group):
// renders shortcuts.ts's SHORTCUTS registry as one row per entry, so this
// and useGlobalShortcuts.ts can never drift out of sync.
import { Fragment } from 'react'
import { Group, Kbd, Modal, Stack, Text } from '@mantine/core'
import { useOs } from '@mantine/hooks'
import { SHORTCUTS, type ShortcutEntry } from './shortcuts'

const WHEN_SUFFIX: Record<NonNullable<ShortcutEntry['when']>, string> = {
  instance: '(on an instance page)',
  console: '(on the console tab)',
}

export function ShortcutsModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const os = useOs()
  const modLabel = os === 'macos' || os === 'ios' ? '⌘' : 'Ctrl'

  return (
    <Modal opened={opened} onClose={onClose} title="Keyboard shortcuts" centered size="sm">
      <Stack gap="xs">
        {SHORTCUTS.map((entry) => {
          const joiner = entry.keys[0] === 'Mod' ? '+' : 'then'
          return (
            <Group key={entry.label} justify="space-between" wrap="nowrap" gap="md">
              <Group gap={6} wrap="wrap">
                <Text size="sm">{entry.label}</Text>
                {entry.when && (
                  <Text size="xs" c="dimmed">
                    {WHEN_SUFFIX[entry.when]}
                  </Text>
                )}
              </Group>
              <Group gap={4} wrap="nowrap">
                {entry.keys.map((key, i) => (
                  <Fragment key={i}>
                    {i > 0 && (
                      <Text size="xs" c="dimmed">
                        {joiner}
                      </Text>
                    )}
                    <Kbd size="sm">{key === 'Mod' ? modLabel : key}</Kbd>
                  </Fragment>
                ))}
              </Group>
            </Group>
          )
        })}
      </Stack>
    </Modal>
  )
}
