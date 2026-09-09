// Header widget (mounted by web/src/layout/Shell.tsx, owned by the
// architect): shows nothing while idle, otherwise a badge with the
// running+queued job count and a popover listing them.
import { Indicator, ActionIcon, Popover, Stack, Group, Text, Badge, Loader, ScrollArea, Divider } from '@mantine/core'
import { IconActivity } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import { fmtAgo } from '../../lib/format'
import { useJobs } from './useJobs'
import { jobTypeLabel } from './jobHelpers'

export function ActivityIndicator() {
  const runningQuery = useJobs({ status: 'running', limit: 20 }, 15_000)
  const queuedQuery = useJobs({ status: 'queued', limit: 20 }, 15_000)

  const running = runningQuery.data ?? []
  const queued = queuedQuery.data ?? []
  const count = running.length + queued.length

  if (count === 0) return null

  return (
    <Popover width={320} position="bottom-end" shadow="md" withArrow>
      <Popover.Target>
        <Indicator label={count} size={16} color="blue" offset={4}>
          <ActionIcon variant="subtle" size="lg" aria-label={`${count} active jobs`}>
            <IconActivity size={18} />
          </ActionIcon>
        </Indicator>
      </Popover.Target>
      <Popover.Dropdown>
        <Stack gap="xs">
          <Text size="sm" fw={600}>
            Active jobs
          </Text>
          <ScrollArea.Autosize mah={280}>
            <Stack gap={6}>
              {running.map((job) => (
                <Group key={job.id} justify="space-between" wrap="nowrap" gap="xs">
                  <div style={{ minWidth: 0 }}>
                    <Text
                      component={Link}
                      to={`/jobs?job=${job.id}`}
                      size="sm"
                      truncate
                      style={{ display: 'block' }}
                    >
                      {job.title || jobTypeLabel(job.type)}
                    </Text>
                    <Text size="xs" c="dimmed" truncate>
                      {job.instance_id || 'global'} · {fmtAgo(job.started_at ?? job.created_at)}
                    </Text>
                  </div>
                  <Badge size="xs" color="blue" variant="light" leftSection={<Loader size={8} color="blue" />}>
                    running
                  </Badge>
                </Group>
              ))}
              {running.length > 0 && queued.length > 0 && <Divider />}
              {queued.map((job) => (
                <Group key={job.id} justify="space-between" wrap="nowrap" gap="xs">
                  <div style={{ minWidth: 0 }}>
                    <Text
                      component={Link}
                      to={`/jobs?job=${job.id}`}
                      size="sm"
                      truncate
                      style={{ display: 'block' }}
                    >
                      {job.title || jobTypeLabel(job.type)}
                    </Text>
                    <Text size="xs" c="dimmed" truncate>
                      {job.instance_id || 'global'} · queued {fmtAgo(job.created_at)}
                    </Text>
                  </div>
                  <Badge size="xs" color="gray" variant="light">
                    queued
                  </Badge>
                </Group>
              ))}
            </Stack>
          </ScrollArea.Autosize>
          <Divider />
          <Text component={Link} to="/jobs" size="xs" c="blue" ta="center">
            View all jobs
          </Text>
        </Stack>
      </Popover.Dropdown>
    </Popover>
  )
}
