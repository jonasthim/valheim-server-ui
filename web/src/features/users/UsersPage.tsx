import { useState } from 'react'
import { ActionIcon, Badge, Button, Group, Skeleton, Table, Text, Title, Tooltip } from '@mantine/core'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { modals } from '@mantine/modals'
import { IconKey, IconPencil, IconTrash, IconUserPlus } from '@tabler/icons-react'
import { api, ApiError } from '../../api/client'
import type { User } from '../../api/types'
import { fmtAgo } from '../../lib/format'
import { notifyError, notifySuccess } from '../../lib/notify'
import { CreateUserModal } from './CreateUserModal'
import { EditUserModal } from './EditUserModal'
import { SetPasswordModal } from './SetPasswordModal'

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
    <>
      <Group justify="space-between" mb="md">
        <Title order={2}>Users</Title>
        <Button leftSection={<IconUserPlus size={16} />} onClick={() => setCreating(true)}>
          New user
        </Button>
      </Group>

      <Table.ScrollContainer minWidth={720}>
        <Table verticalSpacing="sm" highlightOnHover>
          <Table.Thead>
            <Table.Tr>
              <Table.Th>Username</Table.Th>
              <Table.Th>Display name</Table.Th>
              <Table.Th>Email</Table.Th>
              <Table.Th>Role</Table.Th>
              <Table.Th>Status</Table.Th>
              <Table.Th>Identities</Table.Th>
              <Table.Th>Last login</Table.Th>
              <Table.Th />
            </Table.Tr>
          </Table.Thead>
          <Table.Tbody>
            {usersQ.isLoading &&
              Array.from({ length: 3 }).map((_, i) => (
                <Table.Tr key={i}>
                  <Table.Td colSpan={8}>
                    <Skeleton height={20} />
                  </Table.Td>
                </Table.Tr>
              ))}
            {!usersQ.isLoading && users.length === 0 && (
              <Table.Tr>
                <Table.Td colSpan={8}>
                  <Text c="dimmed" ta="center" py="md">
                    No users yet.
                  </Text>
                </Table.Td>
              </Table.Tr>
            )}
            {users.map((u) => (
              <Table.Tr key={u.id}>
                <Table.Td>{u.username}</Table.Td>
                <Table.Td>{u.display_name || '-'}</Table.Td>
                <Table.Td>{u.email || '-'}</Table.Td>
                <Table.Td>
                  <Badge variant="light">{u.role}</Badge>
                </Table.Td>
                <Table.Td>
                  {u.disabled ? (
                    <Badge color="red" variant="light">
                      Disabled
                    </Badge>
                  ) : (
                    <Badge color="green" variant="light">
                      Active
                    </Badge>
                  )}
                </Table.Td>
                <Table.Td>{u.identities.length}</Table.Td>
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
                        u.identities.length > 0
                          ? 'Password managed by the identity provider (SSO-linked account)'
                          : 'Set password'
                      }
                    >
                      <ActionIcon
                        variant="subtle"
                        disabled={u.identities.length > 0}
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
            ))}
          </Table.Tbody>
        </Table>
      </Table.ScrollContainer>

      <CreateUserModal opened={creating} onClose={() => setCreating(false)} />
      {editing && <EditUserModal key={editing.id} opened onClose={() => setEditing(null)} user={editing} />}
      {settingPassword && (
        <SetPasswordModal key={settingPassword.id} opened onClose={() => setSettingPassword(null)} user={settingPassword} />
      )}
    </>
  )
}
