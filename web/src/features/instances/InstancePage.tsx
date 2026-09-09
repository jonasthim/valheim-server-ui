import { Tabs, Title, Stack, Group, Badge } from '@mantine/core'
import { useNavigate, useParams } from 'react-router-dom'
import { useInstance } from './useInstance'
import { INSTANCE_TABS } from './tabs'
import { OverviewTab } from './OverviewTab'
import { ConsoleTab } from './ConsoleTab'
import { ConfigTab } from './ConfigTab'
import { PlayersTab } from './PlayersTab'
import { WorldsTab } from './WorldsTab'
import { BackupsTab } from './BackupsTab'
import { ModsTab } from './ModsTab'
import { SchedulesTab } from './SchedulesTab'

// Instance page with tab routing (/instances/:id/:tab). Owned by WP-11; the tab
// components are owned by WP-11 (Overview/Console/Config), WP-12
// (Players/Worlds/Backups/Schedules) and WP-13 (Mods). Keep this file thin.
export function InstancePage() {
  const { id = '', tab = 'overview' } = useParams()
  const navigate = useNavigate()
  const inst = useInstance(id)
  const state = inst.data?.status.state
  return (
    <Stack>
      <Group>
        <Title order={2}>{inst.data?.name ?? id}</Title>
        {state && <Badge variant="light">{state}</Badge>}
      </Group>
      <Tabs value={tab} onChange={(t) => navigate(`/instances/${id}/${t ?? 'overview'}`)} keepMounted={false}>
        <Tabs.List>
          {INSTANCE_TABS.map((t) => (
            <Tabs.Tab key={t} value={t} tt="capitalize">
              {t}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        <Tabs.Panel value="overview" pt="md"><OverviewTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="console" pt="md"><ConsoleTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="config" pt="md"><ConfigTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="players" pt="md"><PlayersTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="worlds" pt="md"><WorldsTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="backups" pt="md"><BackupsTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="mods" pt="md"><ModsTab id={id} /></Tabs.Panel>
        <Tabs.Panel value="schedules" pt="md"><SchedulesTab id={id} /></Tabs.Panel>
      </Tabs>
    </Stack>
  )
}
