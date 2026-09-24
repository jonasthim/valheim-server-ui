import { lazy, Suspense } from 'react'
import { Badge, Box, Center, Indicator, Loader, NativeSelect, Stack, Tabs } from '@mantine/core'
import { useDocumentTitle } from '@mantine/hooks'
import { useNavigate, useParams } from 'react-router-dom'
import { useAuth } from '../../auth/useAuth'
import { useInstance } from './useInstance'
import { useInstanceStatus } from './instanceActions'
import { INSTANCE_TAB_ICONS, INSTANCE_TAB_LABELS, INSTANCE_TABS, isInstanceTab } from './tabs'
import { useInstanceTabBadges } from './useTabBadges'
import { LifecycleControls } from './LifecycleControls'
import { PendingRestartBanner } from './PendingRestartBanner'
import classes from './InstancePage.module.css'
import { pageTitle } from '../../lib/title'
import { ApiError } from '../../api/client'
import { NotFoundPage } from '../system'
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
import { LoadError, PageHeader, StatusPill } from '../../ui'
import { stateColor, stateLabel } from './instanceHelpers'

// Instance page with tab routing (/instances/:id/:tab). Owned by WP-11; the tab
// components are owned by WP-11 (Overview/Console/Config), WP-12
// (Players/Worlds/Backups/Schedules) and WP-13 (Mods). Keep this file thin.
export function InstancePage() {
  const { id = '', tab = 'overview' } = useParams()
  const navigate = useNavigate()
  const { hasRole } = useAuth()
  const inst = useInstance(id)
  const badges = useInstanceTabBadges(id)
  useDocumentTitle(pageTitle(inst.data?.name ?? id, isInstanceTab(tab) ? INSTANCE_TAB_LABELS[tab] : undefined))
  // The 10s poll and SSE both write ['instances', id, 'status']; fall back to
  // the instance detail's embedded status until the poll resolves once.
  const live = useInstanceStatus(id)
  const status = live.data?.status ?? inst.data?.status
  const state = status?.state
  const config = inst.data?.config

  if (!isInstanceTab(tab)) return <NotFoundPage />

  if (inst.isError) {
    if (inst.error instanceof ApiError && inst.error.status === 404) return <NotFoundPage />
    return (
      <Stack>
        <PageHeader eyebrow={`Instance ${id}`} title={id} />
        <LoadError error={inst.error} title="Instance unavailable" onRetry={() => inst.refetch()} />
      </Stack>
    )
  }

  return (
    <Stack>
      <PageHeader
        eyebrow={`Instance ${id}`}
        title={inst.data?.name ?? id}
        titleAddon={
          state && (
            <StatusPill color={stateColor(state)} pulse={state === 'running' || state === 'starting'}>
              {stateLabel(state)}
            </StatusPill>
          )
        }
        description={config ? `World ${config.world} on port ${config.port}` : undefined}
        actions={
          inst.data && status && hasRole('operator') ? (
            <LifecycleControls id={id} name={inst.data.name} status={status} size="sm" />
          ) : undefined
        }
      />
      {inst.data && status && <PendingRestartBanner id={id} name={inst.data.name} status={status} />}
      <Tabs
        value={tab}
        onChange={(t) => navigate(`/instances/${id}/${t ?? 'overview'}`)}
        keepMounted={false}
        classNames={{ list: classes.list, tab: classes.tab, tabLabel: classes.tabLabel }}
      >
        <Box visibleFrom="sm" style={{ overflowX: 'auto' }}>
          <Tabs.List style={{ flexWrap: 'nowrap' }}>
            {INSTANCE_TABS.map((t) => {
              const Icon = INSTANCE_TAB_ICONS[t]
              const badge = badges[t]
              return (
                <Tabs.Tab
                  key={t}
                  value={t}
                  leftSection={<Icon size={16} stroke={1.75} />}
                  rightSection={
                    badge?.count !== undefined ? (
                      <Badge size="xs" circle color={badge.color}>
                        {badge.count}
                      </Badge>
                    ) : badge?.dot ? (
                      <Indicator color={badge.color} size={6} processing={false} />
                    ) : undefined
                  }
                >
                  {INSTANCE_TAB_LABELS[t]}
                </Tabs.Tab>
              )
            })}
          </Tabs.List>
        </Box>
        <NativeSelect
          hiddenFrom="sm"
          value={tab}
          onChange={(e) => navigate(`/instances/${id}/${e.currentTarget.value}`)}
          data={INSTANCE_TABS.map((t) => ({
            value: t,
            label: badges[t]?.count ? `${INSTANCE_TAB_LABELS[t]} (${badges[t]?.count})` : INSTANCE_TAB_LABELS[t],
          }))}
          aria-label="Instance section"
        />
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
