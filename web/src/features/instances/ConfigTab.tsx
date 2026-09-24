import { useState } from 'react'
import { Box, Button, Group, Skeleton, Stack, Text } from '@mantine/core'
import { useNavigate } from 'react-router-dom'
import { useQueryClient } from '@tanstack/react-query'
import { useAuth } from '../../auth/useAuth'
import { api, ApiError } from '../../api/client'
import type { Instance, UpdateInstanceRequest } from '../../api/types'
import { useUnsavedChanges } from '../../lib/useUnsavedChanges'
import { notifyError, notifySuccess } from '../../lib/notify'
import { SectionCard, StickySaveBar } from '../../ui'
import classes from './ConfigTab.module.css'
import { useInstance } from './useInstance'
import { InstanceConfigForm } from './InstanceConfigForm'
import { useInstanceConfigForm, type InstanceConfigFormSubmit } from './useInstanceConfigForm'
import { mapConfigFieldErrors } from './instanceHelpers'
import { DeleteInstanceModal } from './DeleteInstanceModal'

// Owned by WP-11. Props: the instance id.
export function ConfigTab({ id }: { id: string }) {
  const { hasRole } = useAuth()
  const navigate = useNavigate()
  const inst = useInstance(id)
  const [deleteOpen, setDeleteOpen] = useState(false)

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

  return (
    <Stack>
      {/* Own component so useInstanceConfigForm (a hook) only runs once
          instance data exists — keeps this component's hook order stable
          across the loading/not-found early returns above. */}
      <ConfigTabBody key={id} id={id} instance={instance} />

      {canDelete && (
        <Box maw={720}>
          <SectionCard title="Danger zone" description="Permanently delete this instance. Optionally remove its files too." className={classes.dangerCard}>
            <Group justify="flex-end">
              <Button color="red" variant="outline" onClick={() => setDeleteOpen(true)}>
                Delete instance
              </Button>
            </Group>
          </SectionCard>
        </Box>
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

function ConfigTabBody({ id, instance }: { id: string; instance: Instance }) {
  const { hasRole } = useAuth()
  const qc = useQueryClient()
  const [submitting, setSubmitting] = useState(false)
  const canEdit = hasRole('operator')
  const passwordMasked = instance.config.password === '********'

  const formApi = useInstanceConfigForm({
    mode: 'edit',
    initial: { id: instance.id, name: instance.name, config: instance.config, autostart: instance.status.autostart },
    passwordMasked,
  })
  const unsaved = useUnsavedChanges(formApi.form, { enabled: canEdit })

  async function handleSubmit(values: InstanceConfigFormSubmit, helpers: { setErrors: (e: Record<string, string>) => void }) {
    setSubmitting(true)
    try {
      // Autostart is not part of the edit form (C-2): the Overview switch
      // PATCHes it on its own, so leaving it out keeps that value untouched.
      const payload: UpdateInstanceRequest = {
        name: values.name,
        config: values.config,
      }
      const res = await api.patch<{ instance: Instance }>(`/instances/${id}`, payload)
      qc.setQueryData(['instances', id, 'detail'], res.instance)
      await qc.invalidateQueries({ queryKey: ['instances', 'list'] })
      unsaved.markClean()
      notifySuccess('Changes saved')
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
    <>
      <InstanceConfigForm api={formApi} formId="instance-config-form" readOnly={!canEdit} onSubmit={handleSubmit} />
      {canEdit ? (
        <StickySaveBar
          formId="instance-config-form"
          show={unsaved.dirty}
          dirty={unsaved.dirty}
          saveLabel="Save changes"
          saving={submitting}
          onDiscard={unsaved.discard}
        />
      ) : (
        <Text c="dimmed" size="sm">
          You have read-only access to this configuration.
        </Text>
      )}
    </>
  )
}
