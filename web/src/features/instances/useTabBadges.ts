// Aggregates per-tab attention badges (pending mod updates, players online,
// pending restart) shown in InstancePage's Tabs.List/NativeSelect. Reuses
// queries already cached by the Mods and Overview/Config screens, so this
// hook adds no new network traffic of its own.
import { useModsOverview } from '../mods/useMods'
import { useInstanceStatus } from './instanceActions'
import type { InstanceTab } from './tabs'

type TabBadge = { count?: number; dot?: boolean; color: string }

export function useInstanceTabBadges(id: string): Partial<Record<InstanceTab, TabBadge>> {
  const modsOverview = useModsOverview(id)
  const instanceStatus = useInstanceStatus(id)

  const badges: Partial<Record<InstanceTab, TabBadge>> = {}

  const overview = modsOverview.data
  if (overview) {
    const modUpdateCount = overview.mods.filter((m) => m.update_available).length
    const updateCount = modUpdateCount + (overview.bepinex.update_available ? 1 : 0)
    if (updateCount > 0) badges.mods = { count: updateCount, color: 'spirit' }
  }

  const status = instanceStatus.data?.status
  if (status) {
    if (status.players_online > 0) badges.players = { count: status.players_online, color: 'moss' }
    if (status.pending_restart) badges.config = { dot: true, color: 'straw' }
  }

  return badges
}
