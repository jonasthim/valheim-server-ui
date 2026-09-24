// Module-level store for whether the command palette is open. Lets the
// top-bar button, the Mod+K hotkey (Shell.tsx) and CommandPalette itself
// share one boolean without prop-drilling or a context provider.
import { useSyncExternalStore } from 'react'

let opened = false
const listeners = new Set<() => void>()

function emit() {
  for (const listener of listeners) listener()
}

export function openPalette() {
  opened = true
  emit()
}

export function closePalette() {
  opened = false
  emit()
}

function subscribe(listener: () => void) {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

function getSnapshot() {
  return opened
}

export function usePaletteOpened(): boolean {
  return useSyncExternalStore(subscribe, getSnapshot)
}
