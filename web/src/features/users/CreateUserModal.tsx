import { Button, Group, Modal, PasswordInput, Select, Stack, TextInput } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { api, ApiError } from '../../api/client'
import type { CreateUserRequest, Role } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'
import { ROLE_OPTIONS } from './roles'

interface CreateUserValues {
  username: string
  password: string
  display_name: string
  email: string
  role: Role
}

export function CreateUserModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const qc = useQueryClient()
  const form = useForm<CreateUserValues>({
    initialValues: { username: '', password: '', display_name: '', email: '', role: 'operator' },
    validate: {
      username: (v) => (/^[a-z0-9._-]{2,32}$/.test(v) ? null : 'Lowercase letters, digits, ".", "_" or "-", 2-32 characters'),
      password: (v) => (!v || v.length >= 10 ? null : 'Must be at least 10 characters, or left empty for SSO-only'),
      email: (v) => (!v || /^\S+@\S+\.\S+$/.test(v) ? null : 'Enter a valid email address'),
    },
  })

  const mutation = useMutation({
    mutationFn: (values: CreateUserValues) => {
      const payload: CreateUserRequest = {
        username: values.username,
        role: values.role,
        display_name: values.display_name || undefined,
        email: values.email || undefined,
        password: values.password || undefined,
      }
      return api.post('/users', payload)
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['users'] })
      notifySuccess('User created')
      form.reset()
      onClose()
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        const fields = err.fieldErrors()
        if (Object.keys(fields).length) {
          form.setErrors(fields)
          return
        }
        if (err.code === 'conflict') {
          form.setFieldError('username', 'Username already exists')
          return
        }
      }
      notifyError(err, 'Could not create user')
    },
  })

  return (
    <Modal opened={opened} onClose={onClose} title="New user" centered>
      <form onSubmit={form.onSubmit((values) => mutation.mutate(values))}>
        <Stack gap="sm">
          <TextInput label="Username" autoFocus required {...form.getInputProps('username')} />
          <PasswordInput label="Password" description="Leave empty for SSO-only" {...form.getInputProps('password')} />
          <TextInput label="Display name" {...form.getInputProps('display_name')} />
          <TextInput label="Email" type="email" {...form.getInputProps('email')} />
          <Select
            label="Role"
            data={ROLE_OPTIONS}
            allowDeselect={false}
            required
            {...form.getInputProps('role')}
          />
          <Group justify="flex-end" mt="sm">
            <Button variant="default" onClick={onClose}>
              Cancel
            </Button>
            <Button type="submit" loading={mutation.isPending}>
              Create
            </Button>
          </Group>
        </Stack>
      </form>
    </Modal>
  )
}
