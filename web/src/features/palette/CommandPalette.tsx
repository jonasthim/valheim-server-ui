// Mod+K / top-bar "Search" command palette, hand-built on Mantine's
// Combobox primitive (no @mantine/spotlight dependency) — the same approach
// spotlight itself uses: Combobox.Target wraps the input, Combobox.Options
// renders directly (no Combobox.Dropdown floating positioning) inside a
// Modal, and the combobox store drives keyboard nav over [data-combobox-option].
import { useEffect, useMemo, useState } from 'react'
import { Combobox, Group, Highlight, Kbd, Modal, ScrollArea, Text, TextInput, useCombobox } from '@mantine/core'
import { useDidUpdate } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { filterPaletteActions, type PaletteAction } from './actions'
import { closePalette, usePaletteOpened } from './paletteStore'
import { usePaletteActions } from './usePaletteActions'
import classes from './CommandPalette.module.css'

/** Buckets already-filtered actions by group, preserving first-seen order
 * (which matches the Pages/Instances/This instance/Theme/Help build order),
 * and drops any group left empty by the current filter. */
function groupActions(actions: PaletteAction[]): [string, PaletteAction[]][] {
  const order: string[] = []
  const byGroup = new Map<string, PaletteAction[]>()
  for (const action of actions) {
    if (!byGroup.has(action.group)) {
      byGroup.set(action.group, [])
      order.push(action.group)
    }
    byGroup.get(action.group)!.push(action)
  }
  return order.map((group) => [group, byGroup.get(group)!])
}

export function CommandPalette({ onOpenShortcutsHelp }: { onOpenShortcutsHelp?: () => void }) {
  const opened = usePaletteOpened()
  const [query, setQuery] = useState('')
  const actions = usePaletteActions({ openShortcutsHelp: onOpenShortcutsHelp })
  const byId = useMemo(() => new Map(actions.map((a) => [a.id, a])), [actions])
  const groups = useMemo(() => groupActions(filterPaletteActions(actions, query)), [actions, query])

  const combobox = useCombobox({ onDropdownClose: () => combobox.resetSelectedOption() })

  useEffect(() => {
    if (opened) {
      setQuery('')
      combobox.openDropdown()
    }
    // combobox is a stable store from useCombobox; only re-run when the
    // palette's own opened state flips.
  }, [opened])

  useDidUpdate(() => {
    combobox.selectFirstOption()
  }, [query, opened])

  return (
    <Modal.Root
      opened={opened}
      onClose={closePalette}
      size={560}
      yOffset="12vh"
      radius={10}
      transitionProps={{ transition: 'pop', duration: 120 }}
    >
      <Modal.Overlay />
      <Modal.Content aria-label="Command palette">
        <Modal.Body p={0}>
          <Combobox
            store={combobox}
            withinPortal={false}
            onOptionSubmit={(id) => {
              closePalette()
              byId.get(id)?.run()
            }}
          >
            <Combobox.Target withExpandedAttribute={false}>
              <TextInput
                variant="unstyled"
                size="md"
                data-autofocus
                aria-label="Search commands"
                placeholder="Type a command or search…"
                leftSection={<IconSearch size={18} stroke={1.8} />}
                value={query}
                onChange={(event) => setQuery(event.currentTarget.value)}
                onKeyDown={(event) => {
                  // The combobox's own Escape handling only clears its
                  // internal dropdownOpened flag (Options always renders
                  // regardless -- see the module comment above), which
                  // leaves Mantine's Modal unable to close on Escape itself:
                  // useModal's closeOnEscape skips onClose whenever the
                  // event target carries data-mantine-stop-propagation
                  // (set here by the combobox while its dropdown is open,
                  // which by this component's design is the entire time the
                  // palette is open) -- a convention meant for Escape to
                  // close an inner overlay before an outer one, but with no
                  // Combobox.Dropdown to close here it would otherwise take
                  // two Escape presses. Close the palette directly instead.
                  if (event.key === 'Escape') closePalette()
                }}
                className={classes.search}
              />
            </Combobox.Target>

            <ScrollArea.Autosize mah={360} viewportProps={{ tabIndex: 0 }}>
              <Combobox.Options aria-label="Commands">
                {groups.map(([group, items]) => (
                  <Combobox.Group label={group} key={group} classNames={{ groupLabel: classes.groupLabel }}>
                    {items.map((action) => (
                      <Combobox.Option value={action.id} key={action.id} className={classes.option}>
                        <Group justify="space-between" wrap="nowrap" gap="sm">
                          <div>
                            <Highlight highlight={query} size="sm">
                              {action.label}
                            </Highlight>
                            {action.description && (
                              <Text size="xs" c="dimmed">
                                {action.description}
                              </Text>
                            )}
                          </div>
                          {action.shortcut && (
                            <Kbd size="xs" aria-hidden>
                              {action.shortcut}
                            </Kbd>
                          )}
                        </Group>
                      </Combobox.Option>
                    ))}
                  </Combobox.Group>
                ))}
                {groups.length === 0 && <Combobox.Empty>Nothing found</Combobox.Empty>}
              </Combobox.Options>
            </ScrollArea.Autosize>
          </Combobox>
        </Modal.Body>
      </Modal.Content>
    </Modal.Root>
  )
}
