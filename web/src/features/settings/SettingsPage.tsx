import { useEffect, useState } from 'react'
import { useForm } from '@mantine/form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ActionIcon,
  Alert,
  Anchor,
  Button,
  Card,
  CopyButton,
  Divider,
  Group,
  NumberInput,
  PasswordInput,
  Select,
  Skeleton,
  Stack,
  Switch,
  TagsInput,
  Text,
  TextInput,
  Title,
  Tooltip,
} from '@mantine/core'
import { modals } from '@mantine/modals'
import { IconAlertTriangle, IconCheck, IconCopy, IconPlugConnected, IconRefresh, IconRocket } from '@tabler/icons-react'
import { api, ApiError } from '../../api/client'
import { useAuth } from '../../auth/useAuth'
import type { Settings } from '../../api/types'
import { fmtAgo } from '../../lib/format'
import { notifyError, notifySuccess } from '../../lib/notify'
import { useJobDrawer } from '../jobs'
import {
  ManagerRestartOverlay,
  ReleaseNotesModal,
  UPGRADE_EXPLANATION,
  useCheckAppUpdate,
  useManagerRestartWatch,
  useSystemInfo,
  useUpgradeApp,
} from '../system'
import { DEFAULT_ROLE_OPTIONS } from './options'
import { RoleMappingEditor } from './RoleMappingEditor'

const EMPTY_SETTINGS: Settings = {
  auth: {
    local_login_enabled: true,
    oidc: {
      enabled: false,
      provider_name: 'SSO',
      issuer_url: '',
      client_id: '',
      client_secret: '',
      scopes: ['openid', 'profile', 'email', 'groups'],
      groups_claim: 'groups',
      role_mapping: {},
      default_role: 'viewer',
      auto_create_users: true,
      sync_roles: true,
      redirect_uri: '',
    },
  },
  updates: { check_interval_minutes: 60 },
  thunderstore: { index_refresh_hours: 6 },
  app: { update_check_hours: 6, auto_upgrade: false },
}

interface OidcTestResult {
  ok: boolean
  issuer?: string
  authorization_endpoint?: string
  error?: string
}

export function SettingsPage() {
  const qc = useQueryClient()
  const { hasRole } = useAuth()
  const settingsQ = useQuery({ queryKey: ['settings'], queryFn: () => api.get<Settings>('/settings') })
  const form = useForm<Settings>({ initialValues: EMPTY_SETTINGS })
  const [testResult, setTestResult] = useState<OidcTestResult | null>(null)

  const systemQ = useSystemInfo()
  const checkAppUpdate = useCheckAppUpdate()
  const upgradeApp = useUpgradeApp()
  const { openJob } = useJobDrawer()
  const [notesOpen, setNotesOpen] = useState(false)
  const [upgradeJobId, setUpgradeJobId] = useState<string | undefined>(undefined)
  const { restarting } = useManagerRestartWatch(upgradeJobId)
  const canAdmin = hasRole('admin')

  function confirmUpgradeApp() {
    modals.openConfirmModal({
      title: 'Upgrade Valheim Server UI',
      children: <Text size="sm">{UPGRADE_EXPLANATION}</Text>,
      labels: { confirm: 'Upgrade now', cancel: 'Cancel' },
      onConfirm: () =>
        upgradeApp.mutate(undefined, {
          onSuccess: (res) => {
            setUpgradeJobId(res.job.id)
            openJob(res.job.id)
          },
        }),
    })
  }

  useEffect(() => {
    if (settingsQ.data) form.setValues(settingsQ.data)
    // Re-sync whenever the server copy changes (initial load, or after a save);
    // deliberately not depending on `form` to avoid re-running every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settingsQ.data])

  const saveMutation = useMutation({
    mutationFn: (values: Settings) => api.put<Settings>('/settings', values),
    onSuccess: (data) => {
      qc.setQueryData(['settings'], data)
      notifySuccess('Settings saved')
    },
    onError: (err) => {
      if (err instanceof ApiError) {
        const fields = err.fieldErrors()
        if (Object.keys(fields).length) {
          form.setErrors(fields)
          return
        }
      }
      notifyError(err, 'Could not save settings')
    },
  })

  const testMutation = useMutation({
    mutationFn: () => api.post<OidcTestResult>('/settings/oidc/test', form.values.auth.oidc),
    onSuccess: (data) => setTestResult(data),
    onError: (err) => {
      setTestResult(null)
      notifyError(err, 'Connection test failed')
    },
  })

  if (settingsQ.isLoading) {
    return (
      <Stack maw={720}>
        <Skeleton height={28} width={160} />
        <Skeleton height={260} />
        <Skeleton height={100} />
      </Stack>
    )
  }

  const oidc = form.values.auth.oidc
  const showLockoutWarning = !form.values.auth.local_login_enabled && !oidc.enabled
  const redirectUri = oidc.redirect_uri || ''

  return (
    <Stack maw={720} gap="lg">
      <Title order={2}>Settings</Title>

      <form onSubmit={form.onSubmit((values) => saveMutation.mutate(values))}>
        <Stack gap="lg">
          <Card withBorder padding="lg">
            <Stack gap="md">
              <Title order={4}>Authentication</Title>
              <Switch
                label="Local login enabled"
                description="Allow signing in with a username and password"
                {...form.getInputProps('auth.local_login_enabled', { type: 'checkbox' })}
              />
              {showLockoutWarning && (
                <Alert color="yellow" icon={<IconAlertTriangle size={16} />} title="This will lock everyone out">
                  Local login is off and single sign-on is not enabled. At least one login method must stay on.
                </Alert>
              )}

              <Divider label="Single sign-on (OIDC)" labelPosition="left" />

              <Switch label="Enabled" {...form.getInputProps('auth.oidc.enabled', { type: 'checkbox' })} />

              {oidc.enabled && (
                <Stack gap="sm">
                  <TextInput label="Provider name" {...form.getInputProps('auth.oidc.provider_name')} />
                  <TextInput
                    label="Issuer URL"
                    placeholder="https://idp.example.com"
                    {...form.getInputProps('auth.oidc.issuer_url')}
                  />
                  <TextInput label="Client ID" {...form.getInputProps('auth.oidc.client_id')} />
                  <PasswordInput
                    label="Client secret"
                    placeholder="unchanged"
                    description="Leave blank to keep the current secret"
                    {...form.getInputProps('auth.oidc.client_secret')}
                  />
                  <TagsInput label="Scopes" {...form.getInputProps('auth.oidc.scopes')} />
                  <TextInput label="Groups claim" {...form.getInputProps('auth.oidc.groups_claim')} />

                  <Stack gap={4}>
                    <Text size="sm" fw={500}>
                      Role mapping
                    </Text>
                    <Text size="xs" c="dimmed">
                      Group name → role. The highest matching role wins.
                    </Text>
                    <RoleMappingEditor
                      value={oidc.role_mapping ?? {}}
                      onChange={(v) => form.setFieldValue('auth.oidc.role_mapping', v)}
                      resetToken={settingsQ.dataUpdatedAt}
                    />
                  </Stack>

                  <Select
                    label="Default role"
                    description="Applied to users without a mapped group; deny blocks login"
                    data={DEFAULT_ROLE_OPTIONS}
                    allowDeselect={false}
                    {...form.getInputProps('auth.oidc.default_role')}
                  />
                  <Switch
                    label="Auto-create users"
                    description="Create an account on first successful login"
                    {...form.getInputProps('auth.oidc.auto_create_users', { type: 'checkbox' })}
                  />
                  <Switch
                    label="Sync roles on every login"
                    description="Re-apply the role mapping each time a user signs in"
                    {...form.getInputProps('auth.oidc.sync_roles', { type: 'checkbox' })}
                  />

                  <TextInput
                    label="Redirect URI"
                    description="Register this exact URL at the provider"
                    value={redirectUri}
                    readOnly
                    rightSection={
                      <CopyButton value={redirectUri}>
                        {({ copied, copy }) => (
                          <Tooltip label={copied ? 'Copied' : 'Copy'}>
                            <ActionIcon variant="subtle" onClick={copy} aria-label="Copy redirect URI">
                              {copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
                            </ActionIcon>
                          </Tooltip>
                        )}
                      </CopyButton>
                    }
                  />

                  <Group>
                    <Button
                      type="button"
                      variant="default"
                      leftSection={<IconPlugConnected size={16} />}
                      onClick={() => testMutation.mutate()}
                      loading={testMutation.isPending}
                    >
                      Test connection
                    </Button>
                  </Group>

                  {testResult && (
                    <Alert
                      color={testResult.ok ? 'green' : 'red'}
                      title={testResult.ok ? 'Connection OK' : 'Connection failed'}
                    >
                      {testResult.ok ? (
                        <Stack gap={2}>
                          <Text size="sm">Issuer: {testResult.issuer}</Text>
                          <Text size="sm">Authorization endpoint: {testResult.authorization_endpoint}</Text>
                        </Stack>
                      ) : (
                        <Text size="sm">{testResult.error}</Text>
                      )}
                    </Alert>
                  )}
                </Stack>
              )}
            </Stack>
          </Card>

          <Card withBorder padding="lg">
            <Stack gap="md">
              <Title order={4}>Updates</Title>
              <NumberInput
                label="Check interval (minutes)"
                description="0 disables periodic update checks"
                min={0}
                {...form.getInputProps('updates.check_interval_minutes')}
              />
            </Stack>
          </Card>

          <Card withBorder padding="lg">
            <Stack gap="md">
              <Title order={4}>Thunderstore</Title>
              <NumberInput
                label="Index refresh interval (hours)"
                min={1}
                {...form.getInputProps('thunderstore.index_refresh_hours')}
              />
            </Stack>
          </Card>

          <Card withBorder padding="lg">
            <Stack gap="md">
              <Group justify="space-between">
                <Title order={4}>Application</Title>
                {canAdmin && (
                  <Tooltip label="Check for a new Valheim Server UI release now">
                    <ActionIcon
                      variant="subtle"
                      loading={checkAppUpdate.isPending}
                      onClick={() => checkAppUpdate.mutate()}
                      aria-label="Check for application update"
                    >
                      <IconRefresh size={16} />
                    </ActionIcon>
                  </Tooltip>
                )}
              </Group>

              {systemQ.isLoading && <Skeleton height={80} />}

              {systemQ.data && (
                <Stack gap={4}>
                  <Text size="sm">
                    Current version <Text span fw={600}>{systemQ.data.version}</Text>
                  </Text>
                  {systemQ.data.app_update?.latest_version && (
                    <Text size="sm" c="dimmed">
                      Latest release {systemQ.data.app_update.latest_version}
                      {systemQ.data.app_update.checked_at ? ` · checked ${fmtAgo(systemQ.data.app_update.checked_at)}` : ''}
                      {systemQ.data.app_update.release_url && (
                        <>
                          {' · '}
                          <Anchor href={systemQ.data.app_update.release_url} target="_blank" rel="noreferrer">
                            release notes
                          </Anchor>
                        </>
                      )}
                      {' · '}
                      <Anchor component="button" type="button" onClick={() => setNotesOpen(true)}>
                        what's new
                      </Anchor>
                    </Text>
                  )}
                  {systemQ.data.app_update?.previous_version && (
                    <Text size="xs" c="dimmed">
                      Previous version {systemQ.data.app_update.previous_version} is kept for rollback — run{' '}
                      <Text span ff="monospace">valheim-ui self-upgrade --rollback</Text> on the host to revert.
                    </Text>
                  )}
                </Stack>
              )}

              <NumberInput
                label="Release check interval (hours)"
                description="0 disables periodic checks for new Valheim Server UI releases"
                min={0}
                {...form.getInputProps('app.update_check_hours')}
              />
              <Switch
                label="Auto-upgrade"
                description="Install new releases automatically when no players are online on any server"
                {...form.getInputProps('app.auto_upgrade', { type: 'checkbox' })}
              />

              {canAdmin && (
                <Group>
                  <Button
                    type="button"
                    variant="outline"
                    loading={checkAppUpdate.isPending}
                    onClick={() => checkAppUpdate.mutate()}
                  >
                    Check now
                  </Button>
                  <Tooltip
                    label={systemQ.data?.app_update?.reason ?? 'Self-upgrade unavailable'}
                    disabled={systemQ.data?.app_update?.can_self_upgrade ?? true}
                  >
                    <Button
                      type="button"
                      variant="light"
                      leftSection={<IconRocket size={16} />}
                      disabled={!systemQ.data?.app_update?.update_available || !systemQ.data?.app_update?.can_self_upgrade}
                      loading={upgradeApp.isPending}
                      onClick={confirmUpgradeApp}
                    >
                      Upgrade now
                    </Button>
                  </Tooltip>
                </Group>
              )}
            </Stack>
          </Card>

          <Group justify="flex-end">
            <Button type="submit" loading={saveMutation.isPending}>
              Save settings
            </Button>
          </Group>
        </Stack>
      </form>

      {systemQ.data?.app_update && (
        <ReleaseNotesModal opened={notesOpen} onClose={() => setNotesOpen(false)} appUpdate={systemQ.data.app_update} />
      )}
      <ManagerRestartOverlay visible={restarting} />
    </Stack>
  )
}
