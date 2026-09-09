import { useState } from 'react'
import { Navigate, useNavigate } from 'react-router-dom'
import { Alert, Button, Card, Center, Loader, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { IconAlertCircle } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { api, ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { AuthLayout } from './AuthLayout'

interface SetupValues {
  username: string
  display_name: string
  email: string
  password: string
  confirm: string
}

// POST /auth/setup only succeeds while no users exist; the API 404s afterwards,
// so a direct hit on this page once setup is done bounces to /login.
export function SetupPage() {
  const { status, loading, refresh } = useAuth()
  const navigate = useNavigate()
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  const form = useForm<SetupValues>({
    initialValues: { username: '', display_name: '', email: '', password: '', confirm: '' },
    validate: {
      username: (v) =>
        /^[a-z0-9._-]{2,32}$/.test(v) ? null : 'Lowercase letters, digits, ".", "_" or "-", 2-32 characters',
      email: (v) => (!v || /^\S+@\S+\.\S+$/.test(v) ? null : 'Enter a valid email address'),
      password: (v) => (v.length >= 10 ? null : 'Must be at least 10 characters'),
      confirm: (v, values) => (v === values.password ? null : 'Passwords do not match'),
    },
  })

  if (!loading && status && !status.needs_setup) return <Navigate to="/login" replace />

  async function handleSubmit(values: SetupValues) {
    setSubmitting(true)
    setFormError(null)
    try {
      await api.post<{ user: User }>('/auth/setup', {
        username: values.username,
        password: values.password,
        display_name: values.display_name || undefined,
        email: values.email || undefined,
      })
      await refresh()
      navigate('/', { replace: true })
    } catch (e) {
      if (e instanceof ApiError) {
        const fields = e.fieldErrors()
        if (Object.keys(fields).length) form.setErrors(fields)
        else setFormError(e.message)
      } else {
        setFormError('Something went wrong. Please try again.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <AuthLayout>
      <Card withBorder shadow="sm" padding="xl" w={420}>
        <Stack gap="md">
          <Stack gap={4} align="center">
            <Title order={2}>Create the administrator account</Title>
            <Text c="dimmed" size="sm" ta="center">
              This is the first run: the account you create here becomes the server administrator.
            </Text>
          </Stack>

          {loading ? (
            <Center py="md">
              <Loader />
            </Center>
          ) : (
            <form onSubmit={form.onSubmit(handleSubmit)}>
              <Stack gap="sm">
                {formError && (
                  <Alert color="red" icon={<IconAlertCircle size={16} />}>
                    {formError}
                  </Alert>
                )}
                <TextInput label="Username" autoFocus required {...form.getInputProps('username')} />
                <TextInput label="Display name" {...form.getInputProps('display_name')} />
                <TextInput label="Email" type="email" {...form.getInputProps('email')} />
                <PasswordInput label="Password" required {...form.getInputProps('password')} />
                <PasswordInput label="Confirm password" required {...form.getInputProps('confirm')} />
                <Button type="submit" fullWidth loading={submitting}>
                  Create account
                </Button>
              </Stack>
            </form>
          )}
        </Stack>
      </Card>
    </AuthLayout>
  )
}
