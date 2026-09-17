// Header widget (mounted by web/src/layout/Shell.tsx, owned by the
// architect): always shows the activity icon with a badge for the
// running+queued job count (hidden at zero via Indicator's `disabled`), and a
// popover listing them. Rows open the shared job drawer in place instead of
// navigating away.
import { Indicator, ActionIcon, Popover, Stack, Group, Text, ScrollArea, Divider, UnstyledButton } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { IconActivity } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import { fmtAgo } from '../../lib/format'
import { StatusDot } from '../../ui'
import { useJobs } from './useJobs'
import { useJobDrawer } from './useJobDrawer'
import { jobTypeLabel } from './jobHelpers'

export function ActivityIndicator() {
  const [opened, { toggle, close, set }] = useDisclosure(false)
  const { openJob } = useJobDrawer()
  const runningQuery = useJobs({ status: 'running', limit: 20 }, 15_000)
  const queuedQuery = useJobs({ status: 'queued', limit: 20 }, 15_000)

  const running = runningQuery.data ?? []
  const queued = queuedQuery.data ?? []
  const count = running.length + queued.length

  function openRow(id: string) {
    openJob(id)
    close()
  }

  return (
    <Popover width={320} position="bottom-end" shadow="md" withArrow opened={opened} onChange={set}>
      <Popover.Target>
        <Indicator label={count} size={16} color="frost" offset={4} disabled={count === 0}>
          <ActionIcon variant="subtle" color="gray" size="lg" aria-label={`${count} active jobs`} onClick={toggle}>
            <IconActivity size={18} />
          </ActionIcon>
        </Indicator>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">
          <Text size="sm" fw={600}>
            Active jobs
          </Text>
          {count === 0 ? (
            <Text size="sm" c="dimmed">
              No active jobs
            </Text>
          ) : (
            <ScrollArea.Autosize mah={280}>
              <Stack gap={6}>
                {running.map((job) => (
                  <UnstyledButton key={job.id} onClick={() => openRow(job.id)} style={{ width: '100%', textAlign: 'left' }}>
                    <Group justify="space-between" wrap="nowrap" gap="xs">
                      <div style={{ minWidth: 0 }}>
                        <Text size="sm" truncate style={{ display: 'block' }}>
                          {job.title || jobTypeLabel(job.type)}
                        </Text>
                        <Text size="xs" c="dimmed" truncate>
                          {job.instance_id || 'global'} · {fmtAgo(job.started_at ?? job.created_at)}
                        </Text>
                      </div>
                      <Group gap={6} wrap="nowrap" style={{ flex: 'none' }}>
                        <StatusDot color="frost" pulse />
                        <Text size="xs" c="dimmed">
                          running
                        </Text>
                      </Group>
                    </Group>
                  </UnstyledButton>
                ))}
                {running.length > 0 && queued.length > 0 && <Divider />}
                {queued.map((job) => (
                  <UnstyledButton key={job.id} onClick={() => openRow(job.id)} style={{ width: '100%', textAlign: 'left' }}>
                    <Group justify="space-between" wrap="nowrap" gap="xs">
                      <div style={{ minWidth: 0 }}>
                        <Text size="sm" truncate style={{ display: 'block' }}>
                          {job.title || jobTypeLabel(job.type)}
                        </Text>
                        <Text size="xs" c="dimmed" truncate>
                          {job.instance_id || 'global'} · queued {fmtAgo(job.created_at)}
                        </Text>
                      </div>
                      <Group gap={6} wrap="nowrap" style={{ flex: 'none' }}>
                        <StatusDot color="gray" />
                        <Text size="xs" c="dimmed">
                          queued
                        </Text>
                      </Group>
                    </Group>
                  </UnstyledButton>
                ))}
              </Stack>
            </ScrollArea.Autosize>
          )}
          <Divider />
          <Text component={Link} to="/jobs" size="xs" c="frost" ta="center">
            View all jobs
          </Text>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}
