// Shell's single owner of the global keyboard-shortcut layer (see
// shortcuts.ts for the registry ShortcutsModal renders). Mod+K and Mod+B
// stay on Mantine's useHotkeys since they're plain chords; `?`, the `g …`
// navigation sequences and the instance-page `[`/`]`//` shortcuts run off
// one document keydown listener instead, because useHotkeys can't express
// `?` (Shift+/ produces "?" as event.key, not a physical hotkey string) or
// multi-key sequences. Every branch matches on event.key (not event.code)
// so `[ ] ? /` keep working on Nordic layouts typed via AltGr.
import { useEffect, useRef } from 'react'
import { useHotkeys } from '@mantine/hooks'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import { INSTANCE_TABS, isInstanceTab } from '../features/instances/tabs'
import { IDLE_SEQUENCE, isEditableTarget, isInsideDialog, stepSequence, type SequenceState } from '../lib/keySequence'
import { parseInstanceRoute } from '../lib/routes'
import { GO_SEQUENCES } from './shortcuts'

const GO_KEYS = Object.keys(GO_SEQUENCES)

function clampIndex(idx: number, length: number): number {
  return Math.min(Math.max(idx, 0), length - 1)
}

export interface UseGlobalShortcutsOptions {
  openPalette: () => void
  openHelp: () => void
  toggleSidebar: () => void
}

export function useGlobalShortcuts({ openPalette, openHelp, toggleSidebar }: UseGlobalShortcutsOptions): void {
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { hasRole } = useAuth()
  const sequenceRef = useRef<SequenceState>(IDLE_SEQUENCE)

  // Fires inside inputs too, matching the top-bar Search button.
  useHotkeys([['mod+K', openPalette]], [])
  // Default tagsToIgnore (INPUT/TEXTAREA/SELECT) applies.
  useHotkeys([['mod+B', toggleSidebar]])

  useEffect(() => {
    function handleKeydown(e: KeyboardEvent) {
      if (e.defaultPrevented || isEditableTarget(e.target) || isInsideDialog(e.target)) return

      const plain = !e.ctrlKey && !e.metaKey && !e.altKey
      const current = parseInstanceRoute(pathname)

      if (e.key === '?' && !e.metaKey) {
        e.preventDefault()
        openHelp()
        return
      }

      if ((e.key === '[' || e.key === ']') && !e.metaKey && current && isInstanceTab(current.tab)) {
        const idx = INSTANCE_TABS.indexOf(current.tab)
        const nextIdx = clampIndex(idx + (e.key === '[' ? -1 : 1), INSTANCE_TABS.length)
        e.preventDefault()
        navigate(`/instances/${current.id}/${INSTANCE_TABS[nextIdx]}`)
        return
      }

      if (e.key === '/' && !e.metaKey && current?.tab === 'console') {
        e.preventDefault()
        document.getElementById('console-filter')?.focus()
        return
      }

      if (plain) {
        const { next, matched } = stepSequence(sequenceRef.current, e.key.toLowerCase(), Date.now(), GO_KEYS)
        sequenceRef.current = next
        if (matched) {
          const to = GO_SEQUENCES[matched]
          if (to === '/settings' && !hasRole('admin')) return
          e.preventDefault()
          navigate(to)
        }
      }
    }

    document.addEventListener('keydown', handleKeydown)
    return () => document.removeEventListener('keydown', handleKeydown)
  }, [pathname, navigate, hasRole, openHelp])
}
