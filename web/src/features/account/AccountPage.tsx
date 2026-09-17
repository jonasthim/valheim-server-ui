import {
  Badge,
  Button,
  Code,
  CopyButton,
  Group,
  Loader,
  Modal,
  NumberInput,
  PasswordInput,
  Stack,
  Table,
  Text,
  TextInput,
  Tooltip,
} from '@mantine/core'
import { useForm } from '@mantine/form'
import { useDisclosure } from '@mantine/hooks'
import { useMutation } from '@tanstack/react-query'
import { modals } from '@mantine/modals'
import { IconCheck, IconCopy, IconKey, IconPlus } from '@tabler/icons-react'
import { useAuth } from '../../auth/useAuth'
import { api, ApiError } from '../../api/client'
import { notifyError, notifySuccess } from '../../lib/notify'
import { fmtTime } from '../../lib/format'
import { PageHeader, SectionCard, LoadError } from '../../ui'
import type { APIToken, SessionInfo } from '../../api/types'
import { useRevokeOtherSessions, useRevokeSession, useSessions } from './useSessions'
import { useCreateToken, useRevokeToken, useTokens } from './useTokens'

/** Truncates a long string for table display; the full value goes in a Tooltip. */
function truncate(s: string, n: number): string {
  return s.length > n ? `${s.slice(0, n)}…` : s
}

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

function SessionsCard() {
  const sessionsQ = useSessions()
  const revokeSession = useRevokeSession()
  const revokeOthers = useRevokeOtherSessions()
  const sessions = sessionsQ.data?.sessions ?? []

  function confirmRevoke(session: SessionInfo) {
    modals.openConfirmModal({
      title: 'Sign out this session',
      children: (
        <Text size="sm">
          Sign out the session from <strong>{session.ip || 'this device'}</strong>?
        </Text>
      ),
      labels: { confirm: 'Sign out', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => revokeSession.mutate(session.id),
    })
  }

  function confirmRevokeOthers() {
    modals.openConfirmModal({
      title: 'Sign out everywhere else',
      children: (
        <Text size="sm">
          Sign out every other session on this account? This session stays signed in.
        </Text>
      ),
      labels: { confirm: 'Sign out everywhere else', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => revokeOthers.mutate(),
    })
  }

  return (
    <SectionCard
      title="Sessions"
      actions={
        <Button variant="default" size="xs" disabled={sessions.length <= 1} onClick={confirmRevokeOthers}>
          Sign out everywhere else
        </Button>
      }
    >
      {sessionsQ.isLoading && (
        <Group justify="center" py="md">
          <Loader size="sm" />
        </Group>
      )}
      {sessionsQ.isError && (
        <LoadError error={sessionsQ.error} title="Could not load sessions" onRetry={() => sessionsQ.refetch()} />
      )}
      {!sessionsQ.isLoading && !sessionsQ.isError && (
        <Table.ScrollContainer minWidth={560}>
          <Table verticalSpacing="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>Browser</Table.Th>
                <Table.Th>IP</Table.Th>
                <Table.Th>Signed in</Table.Th>
                <Table.Th>Last seen</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {sessions.map((session) => (
                <Table.Tr key={session.id}>
                  <Table.Td>
                    <Tooltip label={session.user_agent} disabled={session.user_agent.length <= 60}>
                      <Text size="sm">{truncate(session.user_agent, 60) || '-'}</Text>
                    </Tooltip>
                  </Table.Td>
                  <Table.Td>{session.ip || '-'}</Table.Td>
                  <Table.Td>{fmtTime(session.created_at)}</Table.Td>
                  <Table.Td>{fmtTime(session.last_seen_at)}</Table.Td>
                  <Table.Td>
                    {session.current ? (
                      <Badge variant="light">This session</Badge>
                    ) : (
                      <Button
                        size="xs"
                        variant="subtle"
                        color="red"
                        loading={revokeSession.isPending && revokeSession.variables === session.id}
                        onClick={() => confirmRevoke(session)}
                      >
                        Sign out
                      </Button>
                    )}
                  </Table.Td>
                </Table.Tr>
              ))}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      )}
    </SectionCard>
  )
}

interface CreateTokenValues {
  name: string
  expires_in_days: number
}

function CreateTokenModal({ opened, onClose }: { opened: boolean; onClose: () => void }) {
  const createToken = useCreateToken()
  const form = useForm<CreateTokenValues>({
    initialValues: { name: '', expires_in_days: 0 },
    validate: {
      name: (v) => (v.trim().length >= 1 && v.length <= 64 ? null : 'Must be 1-64 characters'),
      expires_in_days: (v) => (v >= 0 && v <= 3650 ? null : 'Must be 0 (never) or 1-3650'),
    },
  })

  function handleClose() {
    form.reset()
    createToken.reset()
    onClose()
  }

  const created = createToken.data

  return (
    <Modal opened={opened} onClose={handleClose} title="New token" radius="lg" centered>
      {created ? (
        <Stack gap="md">
          <Text size="sm">Copy it now — it is not shown again.</Text>
          <Code block>{created.secret}</Code>
          <Group justify="flex-end">
            <CopyButton value={created.secret}>
              {({ copied, copy }) => (
                <Button
                  variant="default"
                  leftSection={copied ? <IconCheck size={16} /> : <IconCopy size={16} />}
                  onClick={copy}
                >
                  {copied ? 'Copied' : 'Copy'}
                </Button>
              )}
            </CopyButton>
            <Button onClick={handleClose}>Done</Button>
          </Group>
        </Stack>
      ) : (
        <form
          onSubmit={form.onSubmit((values) =>
            createToken.mutate(values, {
              onError: (err) => {
                if (err instanceof ApiError) {
                  const fields = err.fieldErrors()
                  if (Object.keys(fields).length) form.setErrors(fields)
                }
              },
            }),
          )}
        >
          <Stack gap="sm">
            <TextInput label="Name" autoFocus required {...form.getInputProps('name')} />
            <NumberInput
              label="Expires in days"
              description="0 = never"
              min={0}
              max={3650}
              {...form.getInputProps('expires_in_days')}
            />
            <Group justify="flex-end">
              <Button variant="default" onClick={handleClose}>
                Cancel
              </Button>
              <Button type="submit" loading={createToken.isPending}>
                Create
              </Button>
            </Group>
          </Stack>
        </form>
      )}
    </Modal>
  )
}

function TokensCard() {
  const tokensQ = useTokens()
  const revokeToken = useRevokeToken()
  const [modalOpened, { open: openModal, close: closeModal }] = useDisclosure(false)
  const tokens = tokensQ.data?.tokens ?? []

  function confirmRevoke(token: APIToken) {
    modals.openConfirmModal({
      title: 'Revoke token',
      children: (
        <Text size="sm">
          Revoke <strong>{token.name}</strong>? Any script or monitor using it stops working immediately.
        </Text>
      ),
      labels: { confirm: 'Revoke', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => revokeToken.mutate(token.id),
    })
  }

  return (
    <SectionCard
      title="API tokens"
      actions={
        <Button variant="default" size="xs" leftSection={<IconPlus size={14} />} onClick={openModal}>
          New token
        </Button>
      }
    >
      {tokensQ.isLoading && (
        <Group justify="center" py="md">
          <Loader size="sm" />
        </Group>
      )}
      {tokensQ.isError && (
        <LoadError error={tokensQ.error} title="Could not load tokens" onRetry={() => tokensQ.refetch()} />
      )}
      {!tokensQ.isLoading && !tokensQ.isError && (
        <Stack gap="sm">
          {tokens.length === 0 ? (
            <Text size="sm" c="dimmed">
              No API tokens yet.
            </Text>
          ) : (
            <Table.ScrollContainer minWidth={640}>
              <Table verticalSpacing="sm">
                <Table.Thead>
                  <Table.Tr>
                    <Table.Th>Name</Table.Th>
                    <Table.Th>Token</Table.Th>
                    <Table.Th>Created</Table.Th>
                    <Table.Th>Last used</Table.Th>
                    <Table.Th>Expires</Table.Th>
                    <Table.Th />
                  </Table.Tr>
                </Table.Thead>
                <Table.Tbody>
                  {tokens.map((token) => (
                    <Table.Tr key={token.id}>
                      <Table.Td>{token.name}</Table.Td>
                      <Table.Td>
                        <Code>{token.prefix}…</Code>
                      </Table.Td>
                      <Table.Td>{fmtTime(token.created_at)}</Table.Td>
                      <Table.Td>{token.last_used_at ? fmtTime(token.last_used_at) : 'never'}</Table.Td>
                      <Table.Td>{token.expires_at ? fmtTime(token.expires_at) : 'never'}</Table.Td>
                      <Table.Td>
                        <Button
                          size="xs"
                          variant="subtle"
                          color="red"
                          loading={revokeToken.isPending && revokeToken.variables === token.id}
                          onClick={() => confirmRevoke(token)}
                        >
                          Revoke
                        </Button>
                      </Table.Td>
                    </Table.Tr>
                  ))}
                </Table.Tbody>
              </Table>
            </Table.ScrollContainer>
          )}
          <Text size="xs" c="dimmed">
            Scripts and monitors authenticate with <Code>Authorization: Bearer &lt;token&gt;</Code> — no cookie or
            CSRF header needed.
          </Text>
        </Stack>
      )}
      <CreateTokenModal opened={modalOpened} onClose={closeModal} />
    </SectionCard>
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
    <Stack gap="xl">
      <PageHeader eyebrow="Account" title="Account" description="Your profile and sign-in details" />

      <Stack gap="md" maw={640}>
        <SectionCard title="Profile">
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
        </SectionCard>

        <SectionCard title="Password">
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
        </SectionCard>

        <SessionsCard />
        <TokensCard />
      </Stack>
    </Stack>
  )
}
