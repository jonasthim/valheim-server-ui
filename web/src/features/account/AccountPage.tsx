import { Badge, Button, Card, Group, Loader, PasswordInput, Stack, Text, Title, Code } from '@mantine/core'
import { useForm } from '@mantine/form'
import { useMutation } from '@tanstack/react-query'
import { IconKey } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { api, ApiError } from '../../api/client'
import { notifyError, notifySuccess } from '../../lib/notify'
import { fmtTime } from '../../lib/format'

interface PasswordValues {
  current_password: string
  new_password: string
  confirm: string
}

function ChangePasswordForm() {
  const form = useForm<PasswordValues>({
    initialValues: { current_password: '', new_password: '', confirm: '' },
    validate: {
      current_password: (v) => (v ? null : 'Required'),
      new_password: (v) => (v.length >= 10 ? null : 'Must be at least 10 characters'),
      confirm: (v, values) => (v === values.new_password ? null : 'Passwords do not match'),
    },
  })

  const mutation = useMutation({
    mutationFn: (values: PasswordValues) =>
      api.put<void>('/auth/password', { current_password: values.current_password, new_password: values.new_password }),
    onSuccess: () => {
      notifySuccess('Password changed')
      form.reset()
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        const fields = err.fieldErrors()
        if (Object.keys(fields).length) form.setErrors(fields)
        else if (err.code === 'invalid_credentials') form.setFieldError('current_password', 'Current password is incorrect')
        else notifyError(err, 'Could not change password')
      } else {
        notifyError(err, 'Could not change password')
      }
    },
  })

  return (
    <form onSubmit={form.onSubmit((values) => mutation.mutate(values))}>
      <Stack gap="sm" maw={420}>
        <PasswordInput
          label="Current password"
          autoComplete="current-password"
          required
          {...form.getInputProps('current_password')}
        />
        <PasswordInput
          label="New password"
          autoComplete="new-password"
          required
          {...form.getInputProps('new_password')}
        />
        <PasswordInput
          label="Confirm new password"
          autoComplete="new-password"
          required
          {...form.getInputProps('confirm')}
        />
        <Group>
          <Button type="submit" leftSection={<IconKey size={16} />} loading={mutation.isPending}>
            Change password
          </Button>
        </Group>
      </Stack>
    </form>
  )
}

export function AccountPage() {
  const { user, loading } = useAuth()

  if (loading || !user) {
    return (
      <Group justify="center" py="xl">
        <Loader />
      </Group>
    )
  }

  return (
    <Stack gap="lg" maw={640}>
      <Title order={2}>Account</Title>

      <Card withBorder padding="lg">
        <Stack gap="sm">
          <Group justify="space-between">
            <Text fw={600}>{user.display_name || user.username}</Text>
            <Badge variant="light">{user.role}</Badge>
          </Group>
          <Group gap="xl">
            <Stack gap={2}>
              <Text size="xs" c="dimmed">
                Username
              </Text>
              <Text size="sm">{user.username}</Text>
            </Stack>
            <Stack gap={2}>
              <Text size="xs" c="dimmed">
                Email
              </Text>
              <Text size="sm">{user.email || '-'}</Text>
            </Stack>
            <Stack gap={2}>
              <Text size="xs" c="dimmed">
                Member since
              </Text>
              <Text size="sm">{fmtTime(user.created_at)}</Text>
            </Stack>
          </Group>
          {user.identities.length > 0 && (
            <Stack gap={4}>
              <Text size="xs" c="dimmed">
                Linked identities
              </Text>
              <Group gap="xs">
                {user.identities.map((idn) => (
                  <Code key={`${idn.provider}:${idn.subject}`}>
                    {idn.provider}:{idn.subject}
                  </Code>
                ))}
              </Group>
            </Stack>
          )}
        </Stack>
      </Card>

      <Card withBorder padding="lg">
        <Stack gap="md">
          <Title order={4}>Password</Title>
          {user.identities.length > 0 ? (
            <Text c="dimmed" size="sm">
              This account is linked to single sign-on; its password is managed by the identity provider and cannot
              be changed here.
            </Text>
          ) : user.has_password ? (
            <ChangePasswordForm />
          ) : (
            <Text c="dimmed" size="sm">
              This account has no local password.
            </Text>
          )}
        </Stack>
      </Card>
    </Stack>
  )
}
