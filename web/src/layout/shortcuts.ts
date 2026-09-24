// Declarative shortcut registry shared by ShortcutsModal.tsx (renders it)
// and useGlobalShortcuts.ts (implements it), so the help modal can never
// drift out of sync with what actually fires.

export interface ShortcutEntry {
  keys: string[]
  label: string
  when?: 'instance' | 'console'
}

export const SHORTCUTS: ShortcutEntry[] = [
  { keys: ['Mod', 'K'], label: 'Open command palette' },
  { keys: ['Mod', 'B'], label: 'Collapse or expand the sidebar' },
  { keys: ['G', 'D'], label: 'Go to Dashboard' },
  { keys: ['G', 'J'], label: 'Go to Jobs' },
  { keys: ['G', 'S'], label: 'Go to Settings' },
  { keys: ['G', 'A'], label: 'Go to Account' },
  { keys: ['['], label: 'Previous instance tab', when: 'instance' },
  { keys: [']'], label: 'Next instance tab', when: 'instance' },
  { keys: ['/'], label: 'Focus the console filter', when: 'console' },
  { keys: ['?'], label: 'Show keyboard shortcuts' },
  { keys: ['Esc'], label: 'Close dialogs and menus' },
]

/** "g d" etc. -> the route to navigate to; keys are space-joined lowercase
 * sequences matching keySequence.ts's `stepSequence` output. */
export const GO_SEQUENCES: Record<string, string> = {
  'g d': '/',
  'g j': '/jobs',
  'g s': '/settings',
  'g a': '/account',
}
