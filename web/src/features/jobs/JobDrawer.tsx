// Right-hand drawer showing one job's header, error/summary, and a live log.
import { useEffect, useRef, useState } from 'react'
import {
  Drawer,
  Stack,
  Group,
  Text,
  Badge,
  Alert,
  Button,
  ActionIcon,
  ScrollArea,
  SimpleGrid,
  Divider,
  Tooltip,
  Loader,
  Anchor,
} from '@mantine/core'
import { useClipboard } from '@mantine/hooks'
import { modals } from '@mantine/modals'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from 'react-router-dom'
import { IconCopy, IconCheck, IconAlertTriangle, IconX, IconArrowBarToDown } from '@tabler/icons-react'
import { api } from '../../api/client'
import { useAuth } from '../../auth/useAuth'
import { onEvent } from '../../events/useEvents'
import { fmtTime } from '../../lib/format'
import { StatusPill } from '../../ui'
import { useJob, useCancelJob } from './useJobs'
import { jobStatusColor, jobTypeLabel, isJobTerminal, isJobCancellable } from './jobHelpers'

export function JobDrawer({ jobId, onClose }: { jobId: string | null; onClose: () => void }) {
  return (
    <Drawer opened={!!jobId} onClose={onClose} position="right" size="lg" title="Job details" aria-label="Job details">
      {/* Keying by jobId remounts the body (fresh log/scroll state) instead of
          reaching for an effect to reset state when the drawer switches jobs. */}
      {jobId && <JobDrawerBody key={jobId} jobId={jobId} />}
    </Drawer>
  )
}

function MetaItem({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <Stack gap={2}>
      <span className="vh-eyebrow">{label}</span>
      <Text size="sm">{value}</Text>
    </Stack>
  )
}

function JobDrawerBody({ jobId }: { jobId: string }) {
  const { hasRole } = useAuth()
  const qc = useQueryClient()
  const clipboard = useClipboard({ timeout: 1000 })
  const cancelJob = useCancelJob()

  const jobQuery = useJob(jobId)
  const job = jobQuery.data?.job

  const logQuery = useQuery({
    queryKey: ['jobs', jobId, 'log'],
    queryFn: () => api.get<{ lines: string[] }>(`/jobs/${jobId}/log`),
  })

  // Lines that have arrived live via SSE since this job was opened, or since
  // it last turned terminal and the log was refetched from disk.
  const [liveLines, setLiveLines] = useState<string[]>([])

  // Keep the job's current status in a ref (updated from an effect, not
  // during render) so the job.log subscription below can read it without
  // resubscribing on every status change.
  const statusRef = useRef(job?.status)
  useEffect(() => {
    statusRef.current = job?.status
  }, [job?.status])

  useEffect(() => {
    return onEvent('job.log', (e) => {
      if (e.job_id !== jobId) return
      const status = statusRef.current
      if (status && !isJobTerminal(status)) {
        setLiveLines((prev) => [...prev, e.line])
      }
    })
  }, [jobId])

  const prevStatusRef = useRef(job?.status)
  useEffect(() => {
    const prev = prevStatusRef.current
    prevStatusRef.current = job?.status
    if (job && prev && prev !== job.status && isJobTerminal(job.status)) {
      // Live-appended lines may be incomplete/racy near the very end; the log
      // file on disk is authoritative once the job is done.
      setLiveLines([])
      void qc.invalidateQueries({ queryKey: ['jobs', jobId, 'log'] })
    }
  }, [job, jobId, qc])

  const lines = [...(logQuery.data?.lines ?? []), ...liveLines]

  const viewportRef = useRef<HTMLDivElement>(null)
  const [follow, setFollow] = useState(true)
  useEffect(() => {
    if (follow && viewportRef.current) {
      viewportRef.current.scrollTo({ top: viewportRef.current.scrollHeight })
    }
  }, [lines.length, follow])

  function handleScrollPositionChange(pos: { x: number; y: number }) {
    const el = viewportRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.clientHeight - pos.y < 24
    setFollow(atBottom)
  }

  function scrollToBottom() {
    setFollow(true)
    if (viewportRef.current) {
      viewportRef.current.scrollTo({ top: viewportRef.current.scrollHeight })
    }
  }

  function confirmCancel() {
    if (!job) return
    if (job.status === 'running') {
      modals.openConfirmModal({
        title: 'Cancel running job',
        children: <Text size="sm">This job is currently running. Cancel it anyway?</Text>,
        labels: { confirm: 'Cancel job', cancel: 'Keep running' },
        confirmProps: { color: 'red' },
        onConfirm: () => cancelJob.mutate(jobId),
      })
    } else {
      cancelJob.mutate(jobId)
    }
  }

  if (!job) {
    return (
      <Group justify="center" py="xl">
        <Loader size="sm" />
      </Group>
    )
  }

  const summaryEntries = job.summary ? Object.entries(job.summary) : []

  return (
    <Stack gap="md">
      <Stack gap={6}>
        <Text fw={600} size="lg">
          {job.title || jobTypeLabel(job.type)}
        </Text>
        <Group gap="xs" wrap="wrap">
          <StatusPill color={jobStatusColor(job.status)} pulse={job.status === 'running'}>
            {job.status}
          </StatusPill>
          <Badge variant="outline">{jobTypeLabel(job.type)}</Badge>
          {job.instance_id && (
            <Anchor component={Link} to={`/instances/${job.instance_id}/overview`} size="sm">
              {job.instance_id}
            </Anchor>
          )}
        </Group>
      </Stack>

      <SimpleGrid cols={{ base: 1, sm: 2 }} spacing="sm">
        <MetaItem label="Instance" value={job.instance_id ?? '-'} />
        <MetaItem label="Requested by" value={job.requested_by ?? '-'} />
        <MetaItem label="Created" value={fmtTime(job.created_at)} />
        <MetaItem label="Started" value={fmtTime(job.started_at)} />
        <MetaItem label="Finished" value={fmtTime(job.finished_at)} />
      </SimpleGrid>

      {job.status === 'failed' && job.error && (
        <Alert color="red" icon={<IconAlertTriangle size={16} />} title="Job failed">
          {job.error}
        </Alert>
      )}

      {summaryEntries.length > 0 && (
        <>
          <Divider label="Summary" labelPosition="left" />
          <Stack gap={4}>
            {summaryEntries.map(([k, v]) => (
              <Group key={k} gap="xs" wrap="nowrap" align="flex-start">
                <Text size="sm" c="dimmed" style={{ flex: 'none', minWidth: 120 }}>
                  {k}
                </Text>
                <Text size="sm" style={{ wordBreak: 'break-word' }}>
                  {typeof v === 'string' ? v : JSON.stringify(v)}
                </Text>
              </Group>
            ))}
          </Stack>
        </>
      )}

      <Divider label="Log" labelPosition="left" />

      <Group justify="space-between">
        <Group gap="xs">
          <Tooltip label={follow ? 'Following new lines' : 'Scroll to bottom to resume following'}>
            <Button
              size="xs"
              variant={follow ? 'filled' : 'default'}
              leftSection={<IconArrowBarToDown size={14} />}
              onClick={scrollToBottom}
            >
              Follow
            </Button>
          </Tooltip>
          <Tooltip label="Copy log to clipboard">
            <ActionIcon
              variant="default"
              aria-label="Copy log to clipboard"
              onClick={() => {
                clipboard.copy(lines.join('\n'))
              }}
            >
              {clipboard.copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
            </ActionIcon>
          </Tooltip>
        </Group>
        {isJobCancellable(job.status) && hasRole('operator') && (
          <Button
            size="xs"
            color="red"
            variant="light"
            leftSection={<IconX size={14} />}
            loading={cancelJob.isPending}
            onClick={confirmCancel}
          >
            Cancel job
          </Button>
        )}
      </Group>

      <ScrollArea.Autosize
        mah={420}
        viewportRef={viewportRef}
        onScrollPositionChange={handleScrollPositionChange}
        className="mono"
        style={{
          background: 'var(--mantine-color-dark-8)',
          borderRadius: 'var(--mantine-radius-md)',
          border: '1px solid var(--vh-border-strong)',
        }}
      >
        <Stack gap={0} p="xs">
          {lines.length === 0 && (
            <Text size="sm" c="dark.2">
              {logQuery.isLoading ? 'Loading log…' : 'No output yet.'}
            </Text>
          )}
          {lines.map((line, i) => (
            <Text key={i} size="xs" c="dark.0" style={{ whiteSpace: 'pre-wrap' }}>
              {line}
            </Text>
          ))}
        </Stack>
      </ScrollArea.Autosize>
    </Stack>
  )
}
