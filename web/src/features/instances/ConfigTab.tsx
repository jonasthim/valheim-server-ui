import { useState } from 'react'
import { Alert, Button, Group, Paper, Skeleton, Stack, Text, Title } from '@mantine/core'
import { useNavigate } from 'react-router-dom'
import { IconAlertTriangle } from '@tabler/icons-react'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../../auth/useAuth'
import { api, ApiError } from '../../api/client'
import type { Instance, UpdateInstanceRequest } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'
import { useInstance } from './useInstance'
import { useRestartInstance } from './instanceActions'
import { InstanceConfigForm, type InstanceConfigFormSubmit } from './InstanceConfigForm'
import { mapConfigFieldErrors } from './instanceHelpers'
import { DeleteInstanceModal } from './DeleteInstanceModal'

// Owned by WP-11. Props: the instance id.
export function ConfigTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const navigate = useNavigate()
  const qc = useQueryClient()
  const inst = useInstance(id)
  const restart = useRestartInstance(id)
  const [submitting, setSubmitting] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)

  const canEdit = hasRole('operator')
  const canDelete = hasRole('admin')

  if (inst.isLoading) {
    return (
      <Stack>
        <Skeleton height={200} />
        <Skeleton height={200} />
      </Stack>
    )
  }

  if (!inst.data) {
    return <Text c="dimmed">Instance not found.</Text>
  }

  const instance = inst.data
  const passwordMasked = instance.config.password === '********'

  async function handleSubmit(values: InstanceConfigFormSubmit, helpers: { setErrors: (e: Record<string, string>) => void }) {
    setSubmitting(true)
    try {
      const payload: UpdateInstanceRequest = {
        name: values.name,
        config: values.config,
        autostart: values.autostart,
      }
      const res = await api.patch<{ instance: Instance }>(`/instances/${id}`, payload)
      qc.setQueryData(['instances', id, 'detail'], res.instance)
      await qc.invalidateQueries({ queryKey: ['instances', 'list'] })
      notifySuccess('Configuration saved')
    } catch (e) {
      if (e instanceof ApiError) {
        const fields = e.fieldErrors()
        if (Object.keys(fields).length) {
          helpers.setErrors(mapConfigFieldErrors(fields))
        } else if (e.code === 'port_in_use') {
          helpers.setErrors({ 'config.port': e.message || 'This port range overlaps another instance' })
        } else {
          notifyError(e, 'Could not save configuration')
        }
      } else {
        notifyError(e, 'Could not save configuration')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <Stack>
      {instance.status.pending_restart && (
        <Alert color="yellow" icon={<IconAlertTriangle size={16} />} title="Restart required to apply">
          <Group justify="space-between" wrap="nowrap">
            <Text size="sm">Configuration changes are saved but will only take effect after a restart.</Text>
            <Button size="xs" color="yellow" variant="filled" loading={restart.isPending} onClick={() => restart.mutate()}>
              Restart now
            </Button>
          </Group>
        </Alert>
      )}

      <InstanceConfigForm
        mode="edit"
        initial={{ id: instance.id, name: instance.name, config: instance.config, autostart: instance.status.autostart }}
        passwordMasked={passwordMasked}
        readOnly={!canEdit}
        submitting={submitting}
        submitLabel="Save changes"
        onSubmit={handleSubmit}
      />

      {canDelete && (
        <Paper withBorder p="md" style={{ borderColor: 'var(--mantine-color-red-6)' }}>
          <Stack gap="sm">
            <Title order={4} c="red">
              Danger zone
            </Title>
            <Group justify="space-between">
              <Text size="sm" c="dimmed">
                Permanently delete this instance. Optionally remove its files too.
              </Text>
              <Button color="red" variant="outline" onClick={() => setDeleteOpen(true)}>
                Delete instance
              </Button>
            </Group>
          </Stack>
        </Paper>
      )}

      <DeleteInstanceModal
        id={id}
        name={instance.name}
        opened={deleteOpen}
        onClose={() => setDeleteOpen(false)}
        onDeleted={() => navigate('/')}
      />
    </Stack>
  )
}
