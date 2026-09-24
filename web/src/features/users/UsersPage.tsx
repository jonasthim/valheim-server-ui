import { useState } from 'react'
import { ActionIcon, Avatar, Badge, Button, Group, Stack, Text, Tooltip } from '@mantine/core'
import { useDocumentTitle } from '@mantine/hooks'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { modals } from '@mantine/modals'
import { IconKey, IconPencil, IconTrash, IconUserPlus, IconUsers } from '@tabler/icons-react'
import { api, ApiError } from '../../api/client'
import { useAuth } from '../../auth/useAuth'
import type { User } from '../../api/types'
import { fmtAgo } from '../../lib/format'
import { notifyError, notifySuccess } from '../../lib/notify'
import { pageTitle } from '../../lib/title'
import { Dash, DataTable, EmptyState, LoadError, PageHeader, SectionCard, StatusPill } from '../../ui'
import type { DataTableColumn } from '../../ui'
import { CreateUserModal } from './CreateUserModal'
import { EditUserModal } from './EditUserModal'
import { SetPasswordModal } from './SetPasswordModal'
import { ROLE_COLORS } from './roles'

export function UsersPage() {
  useDocumentTitle(pageTitle('Users'))
  const qc = useQueryClient()
  const { hasRole } = useAuth()
  const canAdmin = hasRole('admin')
  const usersQ = useQuery({ queryKey: ['users'], queryFn: () => api.get<{ users: User[] }>('/users') })

  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<User | null>(null)
  const [settingPassword, setSettingPassword] = useState<User | null>(null)

  const deleteMutation = useMutation({
    mutationFn: (id: number) => api.del<void>(`/users/${id}`),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['users'] })
      notifySuccess('User deleted')
    },
    onError: (err) => {
      if (err instanceof ApiError && err.code === 'last_admin') {
        notifyError(err, 'Cannot delete user')
        return
      }
      notifyError(err, 'Could not delete user')
    },
  })

  function confirmDelete(user: User) {
    modals.openConfirmModal({
      title: 'Delete user',
      children: (
        <Text size="sm">
          Delete <strong>{user.username}</strong>? This cannot be undone.
        </Text>
      ),
      labels: { confirm: 'Delete', cancel: 'Cancel' },
      confirmProps: { color: 'red' },
      onConfirm: () => deleteMutation.mutate(user.id),
    })
  }

  const users = usersQ.data?.users ?? []

  const columns: DataTableColumn<User>[] = [
    {
      key: 'user',
      header: 'User',
      render: (u) => {
        const initials = (u.display_name || u.username).slice(0, 1).toUpperCase()
        return (
          <Group gap="sm" wrap="nowrap">
            <Avatar radius="md" size={32} color={ROLE_COLORS[u.role]} variant="light">
              {initials}
            </Avatar>
            <div style={{ minWidth: 0 }}>
              <Text size="sm" fw={500} truncate>
                {u.username}
              </Text>
              {u.display_name && (
                <Text size="xs" c="dimmed" truncate>
                  {u.display_name}
                </Text>
              )}
            </div>
          </Group>
        )
      },
    },
    { key: 'email', header: 'Email', render: (u) => u.email || <Dash /> },
    {
      key: 'role',
      header: 'Role',
      render: (u) => (
        <Badge color={ROLE_COLORS[u.role]} variant="light">
          {u.role}
        </Badge>
      ),
    },
    {
      key: 'sso',
      header: 'SSO',
      render: (u) =>
        u.identities.length > 0 ? (
          <Badge color="spirit" variant="light">
            SSO-linked
          </Badge>
        ) : (
          <Text size="sm" c="dimmed">
            Local
          </Text>
        ),
    },
    {
      key: 'status',
      header: 'Status',
      render: (u) => (u.disabled ? <StatusPill color="gray">Disabled</StatusPill> : <StatusPill color="moss">Active</StatusPill>),
    },
    { key: 'last_login', header: 'Last login', render: (u) => fmtAgo(u.last_login_at) },
  ]

  return (
    <Stack gap="lg">
      <PageHeader
        eyebrow="Administration"
        title="Users"
        description="People who can sign in to this UI, and what they're allowed to do."
        actions={
          canAdmin && (
            <Button leftSection={<IconUserPlus size={16} />} onClick={() => setCreating(true)}>
              New user
            </Button>
          )
        }
      />

      <SectionCard flush>
        <DataTable
          columns={columns}
          rows={users}
          rowKey={(u) => u.id}
          loading={usersQ.isLoading}
          error={
            usersQ.isError ? (
              <LoadError error={usersQ.error} title="Could not load users" onRetry={() => usersQ.refetch()} />
            ) : undefined
          }
          empty={<EmptyState icon={<IconUsers size={22} />} title="No users yet." />}
          stickyHeader
          minWidth={800}
          actions={
            canAdmin
              ? (u) => {
                  const ssoLinked = u.identities.length > 0
                  return (
                    <Group gap={4} justify="flex-end" wrap="nowrap">
                      <Tooltip label="Edit">
                        <ActionIcon variant="subtle" onClick={() => setEditing(u)} aria-label={`Edit ${u.username}`}>
                          <IconPencil size={16} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label={ssoLinked ? 'Password managed by the identity provider (SSO-linked account)' : 'Set password'}>
                        <ActionIcon
                          variant="subtle"
                          disabled={ssoLinked}
                          onClick={() => setSettingPassword(u)}
                          aria-label={`Set password for ${u.username}`}
                        >
                          <IconKey size={16} />
                        </ActionIcon>
                      </Tooltip>
                      <Tooltip label="Delete">
                        <ActionIcon variant="subtle" color="red" onClick={() => confirmDelete(u)} aria-label={`Delete ${u.username}`}>
                          <IconTrash size={16} />
                        </ActionIcon>
                      </Tooltip>
                    </Group>
                  )
                }
              : undefined
          }
        />
      </SectionCard>

      <CreateUserModal opened={creating} onClose={() => setCreating(false)} />
      {editing && <EditUserModal key={editing.id} opened onClose={() => setEditing(null)} user={editing} />}
      {settingPassword && (
        <SetPasswordModal key={settingPassword.id} opened onClose={() => setSettingPassword(null)} user={settingPassword} />
      )}
    </Stack>
  )
}
