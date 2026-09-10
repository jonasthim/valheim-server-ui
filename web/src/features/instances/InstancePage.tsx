import { lazy, Suspense } from 'react'
import { Center, Loader, Stack, Tabs } from '@mantine/core'
import { useNavigate, useParams } from 'react-router-dom'
import {
  IconAdjustments,
  IconArchive,
  IconCalendar,
  IconLayoutDashboard,
  IconMap2,
  IconPuzzle,
  IconTerminal2,
  IconUsers,
  IconWorld,
} from '@tabler/icons-react'
import { useInstance } from './useInstance'
import { INSTANCE_TABS, type InstanceTab } from './tabs'
// Tab panels are code-split; keepMounted={false} means each loads on first
// visit, behind the Suspense boundary around the panels below.
const OverviewTab = lazy(() => import('./OverviewTab').then((m) => ({ default: m.OverviewTab })))
const ConsoleTab = lazy(() => import('./ConsoleTab').then((m) => ({ default: m.ConsoleTab })))
const MapTab = lazy(() => import('../map/MapTab').then((m) => ({ default: m.MapTab })))
const ConfigTab = lazy(() => import('./ConfigTab').then((m) => ({ default: m.ConfigTab })))
const PlayersTab = lazy(() => import('./PlayersTab').then((m) => ({ default: m.PlayersTab })))
const WorldsTab = lazy(() => import('./WorldsTab').then((m) => ({ default: m.WorldsTab })))
const BackupsTab = lazy(() => import('./BackupsTab').then((m) => ({ default: m.BackupsTab })))
const ModsTab = lazy(() => import('./ModsTab').then((m) => ({ default: m.ModsTab })))
const SchedulesTab = lazy(() => import('./SchedulesTab').then((m) => ({ default: m.SchedulesTab })))
import { PageHeader, StatusPill } from '../../ui'
import { stateColor, stateLabel } from './instanceHelpers'

const TAB_ICONS: Record<InstanceTab, typeof IconLayoutDashboard> = {
  overview: IconLayoutDashboard,
  console: IconTerminal2,
  map: IconMap2,
  config: IconAdjustments,
  players: IconUsers,
  worlds: IconWorld,
  backups: IconArchive,
  mods: IconPuzzle,
  schedules: IconCalendar,
}

// Instance page with tab routing (/instances/:id/:tab). Owned by WP-11; the tab
// components are owned by WP-11 (Overview/Console/Config), WP-12
// (Players/Worlds/Backups/Schedules) and WP-13 (Mods). Keep this file thin.
export function InstancePage() {
  const { id = '', tab = 'overview' } = useParams()
  const navigate = useNavigate()
  const inst = useInstance(id)
  const state = inst.data?.status.state
  const config = inst.data?.config

  return (
    <Stack>
      <PageHeader
        eyebrow={`Instance · ${id}`}
        title={inst.data?.name ?? id}
        titleAddon={
          state && (
            <StatusPill color={stateColor(state)} pulse={state === 'running' || state === 'starting'}>
              {stateLabel(state)}
            </StatusPill>
          )
        }
        description={config ? `${config.name} · world ${config.world} · port ${config.port}` : undefined}
      />
      <Tabs value={tab} onChange={(t) => navigate(`/instances/${id}/${t ?? 'overview'}`)} keepMounted={false}>
        <div style={{ overflowX: 'auto' }}>
          <Tabs.List style={{ flexWrap: 'nowrap' }}>
            {INSTANCE_TABS.map((t) => {
              const Icon = TAB_ICONS[t]
              return (
                <Tabs.Tab key={t} value={t} tt="capitalize" leftSection={<Icon size={16} stroke={1.8} />}>
                  {t}
                </Tabs.Tab>
              )
            })}
          </Tabs.List>
        </div>
        <Suspense fallback={<Center py="xl"><Loader /></Center>}>
          <Tabs.Panel value="overview" pt="md"><OverviewTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="console" pt="md"><ConsoleTab key={id} id={id} /></Tabs.Panel>
          <Tabs.Panel value="map" pt="md"><MapTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="config" pt="md"><ConfigTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="players" pt="md"><PlayersTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="worlds" pt="md"><WorldsTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="backups" pt="md"><BackupsTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="mods" pt="md"><ModsTab id={id} /></Tabs.Panel>
          <Tabs.Panel value="schedules" pt="md"><SchedulesTab id={id} /></Tabs.Panel>
        </Suspense>
      </Tabs>
    </Stack>
  )
}
