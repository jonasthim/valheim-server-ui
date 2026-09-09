import { Tabs, Title, Stack, Text } from '@mantine/core'
import { useNavigate, useParams } from 'react-router-dom'
import { INSTANCE_TABS } from './tabs'

// Instance page with tab routing (/instances/:id/:tab). Wave 0 shell; WP-11
// owns this file plus Overview/Config/Console tabs, WP-12 Players/Worlds/
// Backups/Schedules, WP-13 Mods. Each tab lives in its own file.
export function InstancePage() {
  const { id = '', tab = 'overview' } = useParams()
  const navigate = useNavigate()
  return (
    <Stack>
      <Title order={2}>{id}</Title>
      <Tabs value={tab} onChange={(t) => navigate(`/instances/${id}/${t ?? 'overview'}`)}>
        <Tabs.List>
          {INSTANCE_TABS.map((t) => (
            <Tabs.Tab key={t} value={t} tt="capitalize">
              {t}
            </Tabs.Tab>
          ))}
        </Tabs.List>
        {INSTANCE_TABS.map((t) => (
          <Tabs.Panel key={t} value={t} pt="md">
            <Text c="dimmed">Tab "{t}" not implemented yet.</Text>
          </Tabs.Panel>
        ))}
      </Tabs>
    </Stack>
  )
}
