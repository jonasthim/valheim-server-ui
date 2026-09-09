import { AppShell, Burger, Group, NavLink, Text, Menu, Avatar, UnstyledButton, Badge } from '@mantine/core'
import { useDisclosure } from '@mantine/hooks'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  IconLayoutDashboard,
  IconListCheck,
  IconUsers,
  IconSettings,
  IconHistory,
  IconLogout,
  IconUserCircle,
} from '@tabler/icons-react'
import { useAuth } from '../auth/useAuth'
import { useEvents } from '../events/useEvents'
import { ActivityIndicator } from '../features/jobs/ActivityIndicator'
import { JobDrawerHost } from '../features/jobs/JobDrawerHost'

const nav = [
  { to: '/', label: 'Dashboard', icon: IconLayoutDashboard, min: 'viewer' as const },
  { to: '/jobs', label: 'Jobs', icon: IconListCheck, min: 'viewer' as const },
  { to: '/users', label: 'Users', icon: IconUsers, min: 'admin' as const },
  { to: '/settings', label: 'Settings', icon: IconSettings, min: 'admin' as const },
  { to: '/audit', label: 'Audit log', icon: IconHistory, min: 'admin' as const },
]

export function Shell() {
  const [opened, { toggle, close }] = useDisclosure()
  const { user, hasRole, logout } = useAuth()
  const loc = useLocation()
  const navigate = useNavigate()
  useEvents(!!user)

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 220, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding="md"
    >
      <AppShell.Header>
        <Group h="100%" px="md" justify="space-between">
          <Group>
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" />
            <Text fw={700} component={Link} to="/" c="inherit" td="none">
              Valheim Server UI
            </Text>
          </Group>
          <Group gap="sm">
            <ActivityIndicator />
            <Menu shadow="md" width={200} position="bottom-end">
              <Menu.Target>
                <UnstyledButton>
                  <Group gap="xs">
                    <Avatar radius="xl" size="sm">
                      {(user?.display_name || user?.username || '?').slice(0, 1).toUpperCase()}
                    </Avatar>
                    <Text size="sm" visibleFrom="sm">
                      {user?.display_name || user?.username}
                    </Text>
                    <Badge size="xs" variant="light" visibleFrom="sm">
                      {user?.role}
                    </Badge>
                  </Group>
                </UnstyledButton>
              </Menu.Target>
              <Menu.Dropdown>
                <Menu.Item leftSection={<IconUserCircle size={16} />} onClick={() => navigate('/account')}>
                  Account
                </Menu.Item>
                <Menu.Item
                  leftSection={<IconLogout size={16} />}
                  onClick={async () => {
                    await logout()
                    navigate('/login')
                  }}
                >
                  Log out
                </Menu.Item>
              </Menu.Dropdown>
            </Menu>
          </Group>
        </Group>
      </AppShell.Header>
      <AppShell.Navbar p="xs">
        {nav
          .filter((n) => hasRole(n.min))
          .map((n) => (
            <NavLink
              key={n.to}
              component={Link}
              to={n.to}
              label={n.label}
              leftSection={<n.icon size={18} />}
              active={n.to === '/' ? loc.pathname === '/' || loc.pathname.startsWith('/instances') : loc.pathname.startsWith(n.to)}
              onClick={close}
            />
          ))}
      </AppShell.Navbar>
      <AppShell.Main>
        <JobDrawerHost>
          <Outlet />
        </JobDrawerHost>
      </AppShell.Main>
    </AppShell>
  )
}
