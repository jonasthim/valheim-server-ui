import { useState } from 'react'
import { Navigate, useNavigate, useSearchParams } from 'react-router-dom'
import { Alert, Button, Card, Center, Divider, Loader, PasswordInput, Stack, Text, TextInput, Title } from '@mantine/core'
import { useForm } from '@mantine/form'
import { IconAlertCircle, IconFingerprint } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { api, ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { AuthLayout } from './AuthLayout'

const ERROR_MESSAGES: Record<string, string> = {
  invalid_credentials: 'Invalid username or password.',
  account_locked: 'Too many failed attempts. Try again in a few minutes.',
  account_disabled: 'This account has been disabled.',
  forbidden: 'Local login is disabled.',
  oidc_disabled: 'Single sign-on is not enabled.',
  oidc_error: 'Single sign-on failed. Please try again or contact an administrator.',
}

function errorMessage(code: string, fallback: string): string {
  return ERROR_MESSAGES[code] ?? fallback
}

interface LoginValues {
  username: string
  password: string
}

// GET /auth/status drives which login methods are shown; after a successful
// login (local or OIDC) the SPA holds a session cookie and /auth/me resolves.
export function LoginPage() {
  const { status, user, loading, refresh } = useAuth()
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const [submitting, setSubmitting] = useState(false)
  const [formError, setFormError] = useState<string | null>(null)

  const rawNext = params.get('next')
  // Same-origin paths only: "//host" and "/\\host" would leave the site.
  const next = rawNext && /^\/(?![/\\])/.test(rawNext) ? rawNext : '/'
  const oidcError = params.get('error')

  const form = useForm<LoginValues>({
    initialValues: { username: '', password: '' },
    validate: {
      username: (v) => (v.trim() ? null : 'Username is required'),
      password: (v) => (v ? null : 'Password is required'),
    },
  })

  if (!loading && status?.needs_setup) return <Navigate to="/setup" replace />
  if (!loading && user) return <Navigate to={next} replace />

  async function handleSubmit(values: LoginValues) {
    setSubmitting(true)
    setFormError(null)
    try {
      await api.post<{ user: User }>('/auth/login', values)
      await refresh()
      navigate(next, { replace: true })
    } catch (e) {
      setFormError(e instanceof ApiError ? errorMessage(e.code, e.message) : 'Something went wrong. Please try again.')
    } finally {
      setSubmitting(false)
    }
  }

  const oidcHref = `/api/v1/auth/oidc/login?next=${encodeURIComponent(next)}`

  return (
    <AuthLayout>
      <Card withBorder shadow="sm" padding="xl" w={380}>
        <Stack gap="md">
          <Stack gap={4} align="center">
            <Title order={2}>Valheim Server UI</Title>
            <Text c="dimmed" size="sm">
              Sign in to manage your servers
            </Text>
          </Stack>

          {oidcError && (
            <Alert color="red" icon={<IconAlertCircle size={16} />} title="Sign-in failed">
              {errorMessage(oidcError, 'Single sign-on failed.')}
            </Alert>
          )}

          {loading ? (
            <Center py="md">
              <Loader />
            </Center>
          ) : (
            <>
              {status?.local_login_enabled && (
                <form onSubmit={form.onSubmit(handleSubmit)}>
                  <Stack gap="sm">
                    {formError && (
                      <Alert color="red" icon={<IconAlertCircle size={16} />}>
                        {formError}
                      </Alert>
                    )}
                    <TextInput
                      label="Username"
                      autoFocus
                      autoComplete="username"
                      required
                      {...form.getInputProps('username')}
                    />
                    <PasswordInput
                      label="Password"
                      autoComplete="current-password"
                      required
                      {...form.getInputProps('password')}
                    />
                    <Button type="submit" fullWidth loading={submitting}>
                      Log in
                    </Button>
                  </Stack>
                </form>
              )}

              {status?.local_login_enabled && status.oidc_enabled && <Divider label="or" labelPosition="center" />}

              {status?.oidc_enabled && (
                <Button component="a" href={oidcHref} variant="default" fullWidth leftSection={<IconFingerprint size={16} />}>
                  Continue with {status.oidc_provider_name || 'SSO'}
                </Button>
              )}

              {status && !status.local_login_enabled && !status.oidc_enabled && (
                <Text c="dimmed" size="sm" ta="center">
                  No login method is available. Contact your administrator.
                </Text>
              )}
            </>
          )}
        </Stack>
      </Card>
    </AuthLayout>
  )
}
