import { Button, Group, Modal, PasswordInput, Stack } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation } from '@tanstack/react-query'
import { api, ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

interface SetPasswordValues {
  new_password: string
  confirm: string
}

export function SetPasswordModal({ opened, onClose, user }: { opened: boolean; onClose: () => void; user: User }) {
  const form = useForm<SetPasswordValues>({
    initialValues: { new_password: '', confirm: '' },
    validate: {
      new_password: (v) => (v.length >= 10 ? null : 'Must be at least 10 characters'),
      confirm: (v, values) => (v === values.new_password ? null : 'Passwords do not match'),
    },
  })

  const mutation = useMutation({
    mutationFn: (values: SetPasswordValues) => api.put<void>(`/users/${user.id}/password`, { new_password: values.new_password }),
    onSuccess: () => {
      notifySuccess(`Password set for ${user.username}`)
      onClose()
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        const fields = err.fieldErrors()
        if (Object.keys(fields).length) {
          form.setErrors(fields)
          return
        }
      }
      notifyError(err, 'Could not set password')
    },
  })

  return (
    <Modal opened={opened} onClose={onClose} title={`Set password for ${user.username}`} centered>
      <form onSubmit={form.onSubmit((values) => mutation.mutate(values))}>
        <Stack gap="sm">
          <PasswordInput label="New password" autoFocus required {...form.getInputProps('new_password')} />
          <PasswordInput label="Confirm password" required {...form.getInputProps('confirm')} />
          <Group justify="flex-end" mt="sm">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={mutation.isPending}>
              Set password
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  )
}
