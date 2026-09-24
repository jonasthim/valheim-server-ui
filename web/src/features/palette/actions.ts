// Pure action-registry builder for the command palette: given live app state
// (role, instances, the current instance route, the theme setter and
// lifecycle callbacks), returns the flat list of actions to render/search.
// Free of React/Mantine so it stays trivial to reason about; usePaletteActions.ts
// wires it up to real hooks and CommandPalette.tsx only renders + filters it.
import type { Instance, Role } from '../../api/types'
import { PAGES } from '../../lib/routes'
import { INSTANCE_TABS, INSTANCE_TAB_LABELS } from '../instances/tabs'
import { canRestart, canStart, canStop, stateLabel } from '../instances/instanceHelpers'

export type PaletteGroup = 'Pages' | 'Instances' | 'This instance' | 'Theme' | 'Help'

export interface PaletteAction {
  id: string
  group: PaletteGroup
  label: string
  description?: string
  keywords?: string[]
  shortcut?: string
  run: () => void
}

export interface PaletteContext {
  hasRole: (min: Role) => boolean
  instances: Instance[]
  current: { id: string; tab: string } | null
  navigate: (to: string) => void
  setColorScheme: (s: 'dark' | 'light' | 'auto') => void
  lifecycle: { start: () => void; stop: () => void; restart: () => void } | null
  openShortcutsHelp?: () => void
  openDocs: (name: string) => void
}

// Only these pages get a "go to" letter shortcut (G + first letter); Users
// and Account have none.
const PAGE_SHORTCUTS: Record<string, string> = {
  '/': 'G D',
  '/jobs': 'G J',
  '/settings': 'G S',
  '/audit': 'G A',
}

export function buildPaletteActions(ctx: PaletteContext): PaletteAction[] {
  const actions: PaletteAction[] = []

  for (const page of PAGES.filter((p) => ctx.hasRole(p.min))) {
    actions.push({
      id: `page:${page.to}`,
      group: 'Pages',
      label: `Go to ${page.label}`,
      shortcut: PAGE_SHORTCUTS[page.to],
      run: () => ctx.navigate(page.to),
    })
  }
  if (ctx.hasRole('admin')) {
    actions.push({
      id: 'page:new-instance',
      group: 'Pages',
      label: 'New instance',
      run: () => ctx.navigate('/instances/new'),
    })
  }

  for (const inst of ctx.instances) {
    actions.push({
      id: `instance:${inst.id}`,
      group: 'Instances',
      label: `Open ${inst.name}`,
      description: stateLabel(inst.status.state),
      keywords: [inst.id],
      run: () => ctx.navigate(`/instances/${inst.id}/overview`),
    })
  }

  const current = ctx.current ? ctx.instances.find((i) => i.id === ctx.current!.id) : undefined
  if (ctx.current && current) {
    for (const tab of INSTANCE_TABS) {
      actions.push({
        id: `tab:${current.id}:${tab}`,
        group: 'This instance',
        label: `${current.name}: ${INSTANCE_TAB_LABELS[tab]}`,
        keywords: [tab],
        run: () => ctx.navigate(`/instances/${current.id}/${tab}`),
      })
    }
    if (ctx.lifecycle && ctx.hasRole('operator')) {
      const lifecycle = ctx.lifecycle
      if (canStart(current.status.state)) {
        actions.push({
          id: 'lifecycle:start',
          group: 'This instance',
          label: `Start ${current.name}`,
          run: lifecycle.start,
        })
      }
      if (canStop(current.status.state)) {
        actions.push({
          id: 'lifecycle:stop',
          group: 'This instance',
          label: `Stop ${current.name}`,
          run: lifecycle.stop,
        })
      }
      if (canRestart(current.status.state)) {
        actions.push({
          id: 'lifecycle:restart',
          group: 'This instance',
          label: `Restart ${current.name}`,
          run: lifecycle.restart,
        })
      }
    }
  }

  actions.push(
    { id: 'theme:dark', group: 'Theme', label: 'Theme: Dark', run: () => ctx.setColorScheme('dark') },
    { id: 'theme:light', group: 'Theme', label: 'Theme: Light', run: () => ctx.setColorScheme('light') },
    { id: 'theme:system', group: 'Theme', label: 'Theme: System', run: () => ctx.setColorScheme('auto') },
  )

  if (ctx.openShortcutsHelp) {
    actions.push({ id: 'help:shortcuts', group: 'Help', label: 'Keyboard shortcuts', run: ctx.openShortcutsHelp })
  }
  actions.push(
    { id: 'help:docs-runbook', group: 'Help', label: 'Docs: Runbook', run: () => ctx.openDocs('RUNBOOK.md') },
    { id: 'help:docs-oidc', group: 'Help', label: 'Docs: Single sign-on', run: () => ctx.openDocs('OIDC.md') },
    { id: 'help:docs-worlds', group: 'Help', label: 'Docs: Worlds', run: () => ctx.openDocs('WORLDS.md') },
  )

  return actions
}

/** Trim + lowercase; empty query keeps everything; otherwise every
 * whitespace-separated term must appear somewhere in the label, description
 * or a keyword. */
export function filterPaletteActions(actions: PaletteAction[], query: string): PaletteAction[] {
  const q = query.trim().toLowerCase()
  if (!q) return actions
  const terms = q.split(/\s+/)
  return actions.filter((a) => {
    const haystack = [a.label, a.description, ...(a.keywords ?? [])]
      .filter(Boolean)
      .join(' ')
      .toLowerCase()
    return terms.every((t) => haystack.includes(t))
  })
}
