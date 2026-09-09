// Jobs list: filters by instance/status, refetches every 10s as an SSE
// fallback, and opens JobDrawer on row click or the `?job=<id>` deep link.
import { useEffect, useRef, useState } from 'react'
import { ActionIcon, Anchor, Group, Select, Skeleton, Stack, Table, Text, TextInput, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { Link, useSearchParams } from 'react-router-dom'
import { IconEye, IconListCheck, IconSearch, IconX } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo, fmtTime } from '../../lib/format'
import type { Job, JobStatus } from '../../api/types'
import { EmptyState, PageHeader, SectionCard, StatusPill } from '../../ui'
import { useJobs, useCancelJob } from './useJobs'
import { jobStatusColor, jobTypeLabel, jobDuration, isJobCancellable } from './jobHelpers'
import { JobDrawerHost } from './JobDrawerHost'
import { useJobDrawer } from './useJobDrawer'

const STATUS_OPTIONS: { value: JobStatus | ''; label: string }[] = [
  { value: '', label: 'All statuses' },
  { value: 'queued', label: 'Queued' },
  { value: 'running', label: 'Running' },
  { value: 'succeeded', label: 'Succeeded' },
  { value: 'failed', label: 'Failed' },
  { value: 'cancelled', label: 'Cancelled' },
]

export function JobsPage() {
  return (
    <JobDrawerHost>
      <JobsPageContent />
    </JobDrawerHost>
  )
}

function JobsPageContent() {
  const { hasRole } = useAuth()
  const { openJob } = useJobDrawer()
  const cancelJob = useCancelJob()
  const [searchParams, setSearchParams] = useSearchParams()

  const [instanceFilter, setInstanceFilter] = useState('')
  const [statusFilter, setStatusFilter] = useState<JobStatus | ''>('')

  const jobsQuery = useJobs({
    instance: instanceFilter || undefined,
    status: statusFilter || undefined,
    limit: 100,
  })

  // Open the drawer once for a `?job=<id>` deep link.
  const openedFromUrl = useRef(false)
  useEffect(() => {
    const jobId = searchParams.get('job')
    if (jobId && !openedFromUrl.current) {
      openedFromUrl.current = true
      openJob(jobId)
    }
  }, [searchParams, openJob])

  function openRow(id: string) {
    openJob(id)
    const next = new URLSearchParams(searchParams)
    next.set('job', id)
    setSearchParams(next, { replace: true })
  }

  function requestCancel(job: Job) {
    if (job.status === 'running') {
      modals.openConfirmModal({
        title: 'Cancel running job',
        children: <Text size="sm">"{job.title || jobTypeLabel(job.type)}" is currently running. Cancel it anyway?</Text>,
        labels: { confirm: 'Cancel job', cancel: 'Keep running' },
        confirmProps: { color: 'red' },
        onConfirm: () => cancelJob.mutate(job.id),
      })
    } else {
      cancelJob.mutate(job.id)
    }
  }

  const jobs = jobsQuery.data ?? []

  return (
    <Stack gap="lg">
      <PageHeader eyebrow="Servers" title="Jobs" description="Background work across every instance — installs, backups, mods and more." />

      <Group gap="sm" wrap="wrap">
        <TextInput
          label="Instance"
          placeholder="Filter by instance id"
          leftSection={<IconSearch size={14} />}
          value={instanceFilter}
          onChange={(e) => setInstanceFilter(e.currentTarget.value)}
          w={220}
        />
        <Select
          label="Status"
          data={STATUS_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
          value={statusFilter}
          onChange={(v) => setStatusFilter((v as JobStatus | '') ?? '')}
          w={180}
          clearable={false}
        />
      </Group>

      <SectionCard flush>
        <Table.ScrollContainer minWidth={900}>
          <Table verticalSpacing="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Status</Table.Th>
                <Table.Th>Type</Table.Th>
                <Table.Th>Title</Table.Th>
                <Table.Th>Instance</Table.Th>
                <Table.Th>Requested by</Table.Th>
                <Table.Th>Created</Table.Th>
                <Table.Th>Duration</Table.Th>
                <Table.Th>Actions</Table.Th>
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {jobsQuery.isLoading &&
                Array.from({ length: 4 }).map((_, i) => (
                  <Table.Tr key={i}>
                    <Table.Td colSpan={8}>
                      <Skeleton height={20} />
                    </Table.Td>
                  </Table.Tr>
                ))}
              {!jobsQuery.isLoading && jobs.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={8}>
                    <EmptyState icon={<IconListCheck size={22} />} title="No jobs match these filters." />
                  </Table.Td>
                </Table.Tr>
              )}
              {jobs.map((job) => (
                <Table.Tr key={job.id} onClick={() => openRow(job.id)} style={{ cursor: 'pointer' }}>
                  <Table.Td>
                    <StatusPill color={jobStatusColor(job.status)} pulse={job.status === 'running'}>
                      {job.status}
                    </StatusPill>
                  </Table.Td>
                  <Table.Td>{jobTypeLabel(job.type)}</Table.Td>
                  <Table.Td>{job.title || <Text c="dimmed">-</Text>}</Table.Td>
                  <Table.Td>
                    {job.instance_id ? (
                      <Anchor
                        component={Link}
                        to={`/instances/${job.instance_id}/overview`}
                        onClick={(e) => e.stopPropagation()}
                        size="sm"
                      >
                        {job.instance_id}
                      </Anchor>
                    ) : (
                      <Text c="dimmed">-</Text>
                    )}
                  </Table.Td>
                  <Table.Td>{job.requested_by || <Text c="dimmed">-</Text>}</Table.Td>
                  <Table.Td>
                    <Text size="sm" title={fmtTime(job.created_at)}>
                      {fmtAgo(job.created_at)}
                    </Text>
                  </Table.Td>
                  <Table.Td>{jobDuration(job)}</Table.Td>
                  <Table.Td>
                    <Group gap={4} wrap="nowrap" justify="flex-end" onClick={(e) => e.stopPropagation()}>
                      <Tooltip label="Open log">
                        <ActionIcon variant="subtle" aria-label="Open log" onClick={() => openRow(job.id)}>
                          <IconEye size={16} />
                        </ActionIcon>
                      </Tooltip>
                      {isJobCancellable(job.status) && hasRole('operator') && (
                        <Tooltip label="Cancel job">
                          <ActionIcon
                            variant="subtle"
                            color="red"
                            aria-label="Cancel job"
                            onClick={() => requestCancel(job)}
                          >
                            <IconX size={16} />
                          </ActionIcon>
                        </Tooltip>
                      )}
                    </Group>
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </SectionCard>
    </Stack>
  )
}
