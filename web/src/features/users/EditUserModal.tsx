import { Button, Divider, Group, Modal, Select, Stack, Switch, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../../api/client'
import type { Role, UpdateUserRequest, User } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'
import { ROLE_OPTIONS } from './roles'

interface EditUserValues {
  display_name: string
  email: string
  role: Role
  disabled: boolean
}

// Rendered with key={user.id} from UsersPage so the form always initializes
// with the correct target user's values.
export function EditUserModal({ opened, onClose, user }: { opened: boolean; onClose: () => void; user: User }) {
  const qc = useQueryClient()
  const form = useForm<EditUserValues>({
    initialValues: {
      display_name: user.display_name ?? '',
      email: user.email ?? '',
      role: user.role,
      disabled: user.disabled,
    },
    validate: {
      email: (v) => (!v || /^\S+@\S+\.\S+$/.test(v) ? null : 'Enter a valid email address'),
    },
  })

  const mutation = useMutation({
    mutationFn: (values: EditUserValues) => {
      const payload: UpdateUserRequest = {
        display_name: values.display_name,
        email: values.email,
        role: values.role,
        disabled: values.disabled,
      }
      return api.patch(`/users/${user.id}`, payload)
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['users'] })
      notifySuccess('User updated')
      onClose()
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        const fields = err.fieldErrors()
        if (Object.keys(fields).length) {
          form.setErrors(fields)
          return
        }
        if (err.code === 'last_admin') {
          notifyError(err, 'Cannot update user')
          return
        }
      }
      notifyError(err, 'Could not update user')
    },
  })

  return (
    <Modal opened={opened} onClose={onClose} title={`Edit ${user.username}`} radius="lg" centered>
      <form onSubmit={form.onSubmit((values) => mutation.mutate(values))}>
        <Stack gap="lg">
          <Stack gap="sm">
            <TextInput label="Display name" autoFocus {...form.getInputProps('display_name')} />
            <TextInput label="Email" type="email" {...form.getInputProps('email')} />
            <Select label="Role" data={ROLE_OPTIONS} allowDeselect={false} required {...form.getInputProps('role')} />
          </Stack>

          <Divider />

          <Switch
            label="Disabled"
            description="Disabled users cannot log in"
            {...form.getInputProps('disabled', { type: 'checkbox' })}
          />

          <Group justify="flex-end">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={mutation.isPending}>
              Save
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  )
}
