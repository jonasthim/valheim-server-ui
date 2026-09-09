import { useState } from 'react'
import { Stack, Title } from '@mantine/core'
import { useNavigate } from 'react-router-dom'
import { api, ApiError } from '../../api/client'
import type { CreateInstanceRequest, Instance, Job } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'
import { useJobDrawer } from '../jobs'
import { InstanceConfigForm, type InstanceConfigFormSubmit } from './InstanceConfigForm'
import { mapConfigFieldErrors } from './instanceHelpers'

const EMPTY_CONFIG: CreateInstanceRequest['config'] = {
  name: '',
  world: 'Dedicated',
  password: '',
  port: 2456,
  public: true,
  crossplay: false,
  preset: '',
  modifiers: {},
  setkeys: [],
  save_interval_sec: 1800,
  game_backups: 4,
  game_backup_short_sec: 7200,
  game_backup_long_sec: 43200,
  extra_args: [],
  bepinex_enabled: false,
  backup_keep_last: 10,
  backup_keep_days: 30,
  backup_before_update: true,
}

export function CreateInstancePage() {
  const navigate = useNavigate()
  const { openJob } = useJobDrawer()
  const [submitting, setSubmitting] = useState(false)

  async function handleSubmit(values: InstanceConfigFormSubmit, helpers: { setErrors: (e: Record<string, string>) => void }) {
    setSubmitting(true)
    try {
      const payload: CreateInstanceRequest = {
        id: values.id,
        name: values.name,
        config: values.config,
        autostart: values.autostart,
        install: values.install,
      }
      const res = await api.post<{ instance: Instance; job?: Job }>('/instances', payload)
      notifySuccess(`Instance "${res.instance.name}" created`)
      if (res.job) openJob(res.job.id)
      navigate(`/instances/${res.instance.id}/overview`)
    } catch (e) {
      if (e instanceof ApiError) {
        const fields = e.fieldErrors()
        if (Object.keys(fields).length) {
          helpers.setErrors(mapConfigFieldErrors(fields))
        } else if (e.code === 'conflict') {
          helpers.setErrors({ id: 'An instance with this ID already exists' })
        } else if (e.code === 'port_in_use') {
          helpers.setErrors({ 'config.port': e.message || 'This port range overlaps another instance' })
        } else {
          notifyError(e, 'Could not create instance')
        }
      } else {
        notifyError(e, 'Could not create instance')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Stack>
      <Title order={2}>New instance</Title>
      <InstanceConfigForm
        mode="create"
        initial={{ name: '', config: EMPTY_CONFIG, autostart: false }}
        submitting={submitting}
        submitLabel="Create instance"
        onSubmit={handleSubmit}
      />
    </Stack>
  )
}
