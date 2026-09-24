// Jobs list: filters by instance/status, refetches every 10s as an SSE
// fallback, and opens JobDrawer on row click or the `?job=<id>` deep link.
import { useEffect, useRef, useState } from 'react'
import { ActionIcon, Anchor, Group, Select, Stack, Text, TextInput, Tooltip } from '@mantine/core'
import { modals } from '@mantine/modals'
import { Link, useSearchParams } from 'react-router-dom'
import { IconEye, IconListCheck, IconSearch, IconX } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { fmtAgo, fmtTime } from '../../lib/format'
import type { Job, JobStatus } from '../../api/types'
import { Dash, DataTable, EmptyState, LoadError, PageHeader, SectionCard, StatusPill, Toolbar, TOOLBAR_INPUT_WIDTH } from '../../ui'
import type { DataTableColumn } from '../../ui'
import { useJobs, useCancelJob } from './useJobs'
import { jobStatusColor, jobTypeLabel, jobDuration, isJobCancellable, jobInstancePath } from './jobHelpers'
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

  const columns: DataTableColumn<Job>[] = [
    {
      key: 'status',
      header: 'Status',
      render: (job) => (
        <StatusPill color={jobStatusColor(job.status)} pulse={job.status === 'running'}>
          {job.status}
        </StatusPill>
      ),
    },
    { key: 'type', header: 'Type', render: (job) => jobTypeLabel(job.type) },
    { key: 'title', header: 'Title', render: (job) => job.title || <Dash /> },
    {
      key: 'instance',
      header: 'Instance',
      render: (job) => {
        const instancePath = jobInstancePath(job)
        return instancePath ? (
          <Anchor component={Link} to={instancePath} onClick={(e) => e.stopPropagation()} size="sm">
            {job.instance_id}
          </Anchor>
        ) : (
          <Dash />
        )
      },
    },
    { key: 'requested_by', header: 'Requested by', render: (job) => job.requested_by || <Dash /> },
    {
      key: 'created',
      header: 'Created',
      render: (job) => (
        <Text size="sm" title={fmtTime(job.created_at)}>
          {fmtAgo(job.created_at)}
        </Text>
      ),
    },
    { key: 'duration', header: 'Duration', render: (job) => jobDuration(job) },
  ]

  return (
    <Stack gap="lg">
      <PageHeader eyebrow="Servers" title="Jobs" description="Background work across every instance — installs, backups, mods and more." />

      <SectionCard flush>
        <Toolbar
          canClear={instanceFilter !== '' || statusFilter !== ''}
          onClear={() => {
            setInstanceFilter('')
            setStatusFilter('')
          }}
        >
          <TextInput
            aria-label="Instance"
            placeholder="Filter by instance id"
            leftSection={<IconSearch size={14} />}
            value={instanceFilter}
            onChange={(e) => setInstanceFilter(e.currentTarget.value)}
            size="sm"
            w={TOOLBAR_INPUT_WIDTH}
          />
          <Select
            aria-label="Status"
            data={STATUS_OPTIONS.map((o) => ({ value: o.value, label: o.label }))}
            value={statusFilter}
            onChange={(v) => setStatusFilter((v as JobStatus | '') ?? '')}
            size="sm"
            w={TOOLBAR_INPUT_WIDTH}
            clearable={false}
          />
        </Toolbar>
        <DataTable
          columns={columns}
          rows={jobs}
          rowKey={(job) => job.id}
          loading={jobsQuery.isLoading}
          error={
            jobsQuery.isError ? (
              <LoadError error={jobsQuery.error} title="Could not load jobs" onRetry={() => jobsQuery.refetch()} />
            ) : undefined
          }
          empty={<EmptyState icon={<IconListCheck size={22} />} title="No jobs match these filters." />}
          stickyHeader
          minWidth={900}
          onRowClick={(job) => openRow(job.id)}
          actions={(job) => (
            <Group gap={4} wrap="nowrap" justify="flex-end">
              <Tooltip label="Open log">
                <ActionIcon variant="subtle" aria-label="Open log" onClick={() => openRow(job.id)}>
                  <IconEye size={16} />
                </ActionIcon>
              </Tooltip>
              {isJobCancellable(job.status) && hasRole('operator') && (
                <Tooltip label="Cancel job">
                  <ActionIcon variant="subtle" color="red" aria-label="Cancel job" onClick={() => requestCancel(job)}>
                    <IconX size={16} />
                  </ActionIcon>
                </Tooltip>
              )}
            </Group>
          )}
        />
      </SectionCard>
    </Stack>
  )
}
