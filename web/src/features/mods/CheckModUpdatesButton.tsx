// "Check for updates" for the Mods tab: refreshes the Thunderstore index
// (a global thunderstore_refresh job), waits for it to finish, then
// recomputes each installed mod's update_available and reports the count.
import { useEffect, useRef, useState } from 'react'
import { Button } from '@mantine/core'
import { IconRefresh } from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import { notifyError, notifySuccess } from '../../lib/notify'
import type { Job, ModsOverview } from '../../api/types'
import { useJob } from '../jobs'

export function CheckModUpdatesButton({ id }: { id: string }) {
  const qc = useQueryClient()
  const [jobId, setJobId] = useState<string | undefined>()
  const job = useJob(jobId)
  const handled = useRef<string | undefined>(undefined)

  const start = useMutation({
    mutationFn: () => api.post<{ job: Job }>('/thunderstore/refresh'),
    onSuccess: (res) => setJobId(res.job.id),
    onError: (err) => notifyError(err, 'Could not check for updates'),
  })

  const current = job.data?.job
  const terminal =
    current !== undefined &&
    (current.status === 'succeeded' || current.status === 'failed' || current.status === 'cancelled')

  useEffect(() => {
    // Fire the completion side-effects once per finished job. The refresh
    // job carries no instance id, so its own SSE completion does not
    // invalidate this instance's mods — do it here so the update_available
    // flags recompute against the fresh index.
    if (!current || !terminal || handled.current === current.id) return
    handled.current = current.id
    if (current.status === 'succeeded') {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] }).then(() => {
        const overview = qc.getQueryData<ModsOverview>(['instances', id, 'mods'])
        const n = overview?.mods.filter((m) => m.update_available).length ?? 0
        notifySuccess(
          n === 0 ? 'All mods are up to date.' : `${n} mod update${n === 1 ? '' : 's'} available.`,
          'Checked for updates',
        )
      })
    } else {
      notifyError(
        new Error(current.error || 'the index refresh did not complete'),
        'Could not check for updates',
      )
    }
  }, [current, terminal, id, qc])

  const running = start.isPending || (jobId !== undefined && !terminal)

  return (
    <Button
      variant="default"
      leftSection={<IconRefresh size={16} />}
      loading={running}
      onClick={() => start.mutate()}
    >
      Check for updates
    </Button>
  )
}
