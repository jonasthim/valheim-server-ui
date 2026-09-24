// Wires buildPaletteActions (pure) up to real app state: auth role, the
// instance list, the current instance route, the theme setter, the job
// drawer and the current instance's lifecycle mutations. CommandPalette.tsx
// calls this and only handles rendering + keyboard nav.
import { useMemo } from 'react'
import { useMantineColorScheme } from '@mantine/core'
import { useLocation, useNavigate } from 'react-router-dom'
import { useAuth } from '../../auth/useAuth'
import { docsUrl } from '../../lib/docs'
import { parseInstanceRoute } from '../../lib/routes'
import { useJobDrawer } from '../jobs'
import { useRestartInstance, useStartInstance, useStopInstance } from '../instances/instanceActions'
import { useInstances } from '../instances/useInstances'
import { openConfirmRestart, openConfirmStop } from '../instances/openConfirmLifecycle'
import { buildPaletteActions, type PaletteAction } from './actions'

export function usePaletteActions(opts: { openShortcutsHelp?: () => void } = {}): PaletteAction[] {
  const { openShortcutsHelp } = opts
  const { hasRole } = useAuth()
  const navigate = useNavigate()
  const { pathname } = useLocation()
  const { data: instances = [] } = useInstances()
  const { setColorScheme } = useMantineColorScheme()
  const { openJob } = useJobDrawer()

  // Memoized so its identity only changes when the route actually does --
  // `current` is passed whole into the buildPaletteActions deps below, and
  // a fresh object every render would defeat that memo entirely.
  const current = useMemo(() => parseInstanceRoute(pathname), [pathname])
  const inst = instances.find((i) => i.id === current?.id)

  const start = useStartInstance(current?.id ?? '')
  const stop = useStopInstance(current?.id ?? '')
  const restart = useRestartInstance(current?.id ?? '')

  const openDocs = (name: string) => window.open(docsUrl(name), '_blank', 'noopener')

  return useMemo(() => {
    // Built inside the memo (rather than as an outer `const`) so its
    // per-render object literal never itself has to be a dependency --
    // only the stable-ish hook values it closes over do.
    const lifecycle = inst
      ? {
          start: () => start.mutate(),
          stop: () =>
            openConfirmStop({
              instanceName: inst.name,
              playersOnline: inst.status.players_online,
              onConfirm: () => stop.mutate(),
            }),
          restart: () =>
            openConfirmRestart({
              instanceName: inst.name,
              playersOnline: inst.status.players_online,
              onConfirm: () => restart.mutate(0, { onSuccess: (r) => r.job && openJob(r.job.id) }),
            }),
        }
      : null

    return buildPaletteActions({
      hasRole,
      instances,
      current,
      navigate,
      setColorScheme,
      lifecycle,
      openShortcutsHelp,
      openDocs,
    })
  }, [hasRole, instances, current, inst, start, stop, restart, openJob, openShortcutsHelp, navigate, setColorScheme])
}
