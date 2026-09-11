import { useState } from 'react'
import { ActionIcon, Avatar, Badge, Button, Group, Skeleton, Stack, Table, Text, Tooltip } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { modals } from '@mantine/modals'
import { IconKey, IconPencil, IconTrash, IconUserPlus, IconUsers } from '@tabler/icons-react'
import { api, ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { fmtAgo } from '../../lib/format'
import { notifyError, notifySuccess } from '../../lib/notify'
import { EmptyState, LoadError, PageHeader, SectionCard, StatusPill } from '../../ui'
import { CreateUserModal } from './CreateUserModal'
import { EditUserModal } from './EditUserModal'
import { SetPasswordModal } from './SetPasswordModal'
import { ROLE_COLORS } from './roles'

export function UsersPage() {
  const qc = useQueryClient()
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

  return (
    <Stack gap="lg">
      <PageHeader
        eyebrow="Administration"
        title="Users"
        description="People who can sign in to this UI, and what they're allowed to do."
        actions={
          <Button leftSection={<IconUserPlus size={16} />} onClick={() => setCreating(true)}>
            New user
          </Button>
        }
      />

      <SectionCard flush>
        <Table.ScrollContainer minWidth={800}>
          <Table verticalSpacing="sm">
            <Table.Thead>
              <Table.Tr>
                <Table.Th>User</Table.Th>
                <Table.Th>Email</Table.Th>
                <Table.Th>Role</Table.Th>
                <Table.Th>SSO</Table.Th>
                <Table.Th>Status</Table.Th>
                <Table.Th>Last login</Table.Th>
                <Table.Th />
              </Table.Tr>
            </Table.Thead>
            <Table.Tbody>
              {usersQ.isLoading &&
                Array.from({ length: 3 }).map((_, i) => (
                  <Table.Tr key={i}>
                    <Table.Td colSpan={7}>
                      <Skeleton height={20} />
                    </Table.Td>
                  </Table.Tr>
                ))}
              {usersQ.isError && (
                <Table.Tr>
                  <Table.Td colSpan={7}>
                    <LoadError
                      error={usersQ.error}
                      title="Could not load users"
                      onRetry={() => usersQ.refetch()}
                    />
                  </Table.Td>
                </Table.Tr>
              )}
              {!usersQ.isLoading && !usersQ.isError && users.length === 0 && (
                <Table.Tr>
                  <Table.Td colSpan={7}>
                    <EmptyState icon={<IconUsers size={22} />} title="No users yet." />
                  </Table.Td>
                </Table.Tr>
              )}
              {users.map((u) => {
                const initials = (u.display_name || u.username).slice(0, 1).toUpperCase()
                const ssoLinked = u.identities.length > 0
                return (
                  <Table.Tr key={u.id}>
                    <Table.Td>
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
                    </Table.Td>
                    <Table.Td>{u.email || '-'}</Table.Td>
                    <Table.Td>
                      <Badge color={ROLE_COLORS[u.role]} variant="light">
                        {u.role}
                      </Badge>
                    </Table.Td>
                    <Table.Td>
                      {ssoLinked ? (
                        <Badge color="spirit" variant="light">
                          SSO-linked
                        </Badge>
                      ) : (
                        <Text size="sm" c="dimmed">
                          Local
                        </Text>
                      )}
                    </Table.Td>
                    <Table.Td>
                      {u.disabled ? (
                        <StatusPill color="gray">Disabled</StatusPill>
                      ) : (
                        <StatusPill color="moss">Active</StatusPill>
                      )}
                    </Table.Td>
                    <Table.Td>{fmtAgo(u.last_login_at)}</Table.Td>
                    <Table.Td>
                      <Group gap={4} justify="flex-end" wrap="nowrap">
                        <Tooltip label="Edit">
                          <ActionIcon variant="subtle" onClick={() => setEditing(u)} aria-label={`Edit ${u.username}`}>
                            <IconPencil size={16} />
                          </ActionIcon>
                        </Tooltip>
                        <Tooltip
                          label={
                            ssoLinked
                              ? 'Password managed by the identity provider (SSO-linked account)'
                              : 'Set password'
                          }
                        >
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
                          <ActionIcon
                            variant="subtle"
                            color="red"
                            onClick={() => confirmDelete(u)}
                            aria-label={`Delete ${u.username}`}
                          >
                            <IconTrash size={16} />
                          </ActionIcon>
                        </Tooltip>
                      </Group>
                    </Table.Td>
                  </Table.Tr>
                )
              })}
            </Table.Tbody>
          </Table>
        </Table.ScrollContainer>
      </SectionCard>

      <CreateUserModal opened={creating} onClose={() => setCreating(false)} />
      {editing && <EditUserModal key={editing.id} opened onClose={() => setEditing(null)} user={editing} />}
      {settingPassword && (
        <SetPasswordModal key={settingPassword.id} opened onClose={() => setSettingPassword(null)} user={settingPassword} />
      )}
    </Stack>
  )
}
