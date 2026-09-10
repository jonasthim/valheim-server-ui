import {
  ActionIcon,
  AppShell,
  Avatar,
  Center,
  Badge,
  Burger,
  Group,
  Loader,
  Menu,
  NavLink,
  ScrollArea,
  Stack,
  Text,
  Tooltip,
  UnstyledButton,
  useComputedColorScheme,
  useMantineColorScheme,
} from '@mantine/core'
import { Suspense } from 'react'
import { useDisclosure } from '@mantine/hooks'
import { Link, Outlet, useLocation, useNavigate } from 'react-router-dom'
import {
  IconChevronRight,
  IconHistory,
  IconLayoutDashboard,
  IconListCheck,
  IconLogout,
  IconMoonStars,
  IconSettings,
  IconSun,
  IconUserCircle,
  IconUsers,
} from '@tabler/icons-react'
import { useAuth } from '../auth/useAuth'
import { useEvents } from '../events/useEvents'
import { ActivityIndicator } from '../features/jobs/ActivityIndicator'
import { JobDrawerHost } from '../features/jobs/JobDrawerHost'
import { useSystemInfo } from '../features/system'
import { BrandMark, ErrorBoundary } from '../ui'
import classes from './Shell.module.css'

type Role = 'viewer' | 'operator' | 'admin'
interface NavItem {
  to: string
  label: string
  icon: typeof IconLayoutDashboard
  min: Role
}
const NAV_GROUPS: { label: string; items: NavItem[] }[] = [
  {
    label: 'Servers',
    items: [
      { to: '/', label: 'Dashboard', icon: IconLayoutDashboard, min: 'viewer' },
      { to: '/jobs', label: 'Jobs', icon: IconListCheck, min: 'viewer' },
    ],
  },
  {
    label: 'Administration',
    items: [
      { to: '/users', label: 'Users', icon: IconUsers, min: 'admin' },
      { to: '/settings', label: 'Settings', icon: IconSettings, min: 'admin' },
      { to: '/audit', label: 'Audit log', icon: IconHistory, min: 'admin' },
    ],
  },
]

function ColorSchemeToggle() {
  const { setColorScheme } = useMantineColorScheme()
  const computed = useComputedColorScheme('dark')
  const next = computed === 'dark' ? 'light' : 'dark'
  return (
    <Tooltip label={`Switch to ${next} mode`}>
      <ActionIcon variant="subtle" color="gray" size="lg" aria-label={`Switch to ${next} mode`} onClick={() => setColorScheme(next)}>
        {computed === 'dark' ? <IconSun size={18} /> : <IconMoonStars size={18} />}
      </ActionIcon>
    </Tooltip>
  )
}

export function Shell() {
  const [opened, { toggle, close }] = useDisclosure()
  const { user, hasRole, logout } = useAuth()
  const loc = useLocation()
  const navigate = useNavigate()
  const system = useSystemInfo()
  useEvents(!!user)

  const initials = (user?.display_name || user?.username || '?').slice(0, 1).toUpperCase()

  const userMenu = (
    <Menu shadow="md" width={220} position="top-start" withinPortal>
      <Menu.Target>
        <UnstyledButton className={classes.userButton} aria-label="Account menu">
          <Group gap="sm" wrap="nowrap">
            <Avatar radius="md" size={34} color="ember" variant="light">
              {initials}
            </Avatar>
            <Stack gap={0} style={{ minWidth: 0, flex: 1 }}>
              <Text size="sm" fw={600} truncate>
                {user?.display_name || user?.username}
              </Text>
              <Text size="xs" c="dimmed" truncate tt="capitalize">
                {user?.role}
              </Text>
            </Stack>
            <IconChevronRight size={16} style={{ opacity: 0.5 }} />
          </Group>
        </UnstyledButton>
      </Menu.Target>
      <Menu.Dropdown>
        <Menu.Label>Signed in as {user?.username}</Menu.Label>
        <Menu.Item leftSection={<IconUserCircle size={16} />} onClick={() => navigate('/account')}>
          Account
        </Menu.Item>
        <Menu.Divider />
        <Menu.Item
          color="red"
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
  )

  return (
    <AppShell
      header={{ height: 56 }}
      navbar={{ width: 250, breakpoint: 'sm', collapsed: { mobile: !opened } }}
      padding={{ base: 'md', md: 'xl' }}
    >
      <AppShell.Header className={classes.header}>
        <Group h="100%" px="md" justify="space-between" wrap="nowrap">
          <Group gap="sm" wrap="nowrap">
            <Burger opened={opened} onClick={toggle} hiddenFrom="sm" size="sm" aria-label="Toggle navigation" />
            <Link to="/" className={classes.brand} onClick={close}>
              <BrandMark size={26} />
              <Stack gap={0}>
                <span className={classes.brandName}>Valheim</span>
                <span className={classes.brandSub}>Server UI</span>
              </Stack>
            </Link>
          </Group>
          <Group gap="xs" wrap="nowrap">
            <ActivityIndicator />
            <ColorSchemeToggle />
          </Group>
        </Group>
      </AppShell.Header>

      <AppShell.Navbar className={classes.navbar}>
        <ScrollArea style={{ flex: 1 }} px="xs" py="xs">
          {NAV_GROUPS.map((group) => {
            const items = group.items.filter((n) => hasRole(n.min))
            if (items.length === 0) return null
            return (
              <div key={group.label}>
                <span className={classes.groupLabel}>{group.label}</span>
                <Stack gap={2}>
                  {items.map((n) => {
                    const active =
                      n.to === '/' ? loc.pathname === '/' || loc.pathname.startsWith('/instances') : loc.pathname.startsWith(n.to)
                    return (
                      <NavLink
                        key={n.to}
                        component={Link}
                        to={n.to}
                        label={n.label}
                        className={classes.link}
                        leftSection={
                          <span className={classes.linkIcon}>
                            <n.icon size={18} stroke={1.8} />
                          </span>
                        }
                        active={active}
                        variant="subtle"
                        onClick={close}
                      />
                    )
                  })}
                </Stack>
              </div>
            )
          })}
        </ScrollArea>
        <div className={classes.navFooter}>
          {userMenu}
          <Group justify="space-between" px={8} pt={6}>
            <Text size="xs" c="dimmed">
              {system.data?.version ? `v${system.data.version.replace(/^v/, '')}` : ''}
            </Text>
            {system.data?.app_update?.update_available && (
              <Badge size="xs" color="ember" variant="filled" component={Link} to="/settings" style={{ cursor: 'pointer' }}>
                Update
              </Badge>
            )}
          </Group>
        </div>
      </AppShell.Navbar>

      <AppShell.Main className={classes.main}>
        <div className={classes.content}>
          <JobDrawerHost>
            <ErrorBoundary key={loc.pathname}>
              <Suspense fallback={<Center h="50vh"><Loader /></Center>}>
                <Outlet />
              </Suspense>
            </ErrorBoundary>
          </JobDrawerHost>
        </div>
      </AppShell.Main>
    </AppShell>
  )
}
