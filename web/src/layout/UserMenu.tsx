import { Avatar, Menu, Text, UnstyledButton, useMantineColorScheme } from '@mantine/core'
import { IconCheck, IconDeviceDesktop, IconLogout, IconMoonStars, IconSun, IconUserCircle } from '@tabler/icons-react'
import { useNavigate } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import classes from './Shell.module.css'

// Top-bar avatar menu (TopBar.tsx): account link, theme switcher (dark /
// light / system), log out. Replaces the old sidebar-footer user menu and
// the separate sun/moon toggle.
export function UserMenu() {
  const { user, logout } = useAuth()
  const navigate = useNavigate()
  const { colorScheme, setColorScheme } = useMantineColorScheme()

  const initials = (user?.display_name || user?.username || '?').slice(0, 1).toUpperCase()

  return (
    <Menu shadow="md" width={220} position="bottom-end" withinPortal>
      <Menu.Target>
        <UnstyledButton aria-label="Account menu" className={classes.avatarButton}>
          <Avatar radius="xl" size={28} color="ember" variant="light">
            {initials}
          </Avatar>
        </UnstyledButton>
      </Menu.Target>
      <Menu.Dropdown>
        <div className={classes.menuHeader}>
          <Text size="sm" fw={600} truncate>
            {user?.display_name || user?.username}
          </Text>
          <Text size="xs" c="dimmed" truncate>
            Signed in as {user?.username}
          </Text>
        </div>
        <Menu.Divider />
        <Menu.Item leftSection={<IconUserCircle size={16} />} onClick={() => navigate('/account')}>
          Account
        </Menu.Item>
        <Menu.Divider />
        <Menu.Label>Theme</Menu.Label>
        <Menu.Item
          leftSection={<IconMoonStars size={16} />}
          rightSection={colorScheme === 'dark' ? <IconCheck size={14} aria-hidden /> : undefined}
          onClick={() => setColorScheme('dark')}
        >
          Dark
        </Menu.Item>
        <Menu.Item
          leftSection={<IconSun size={16} />}
          rightSection={colorScheme === 'light' ? <IconCheck size={14} aria-hidden /> : undefined}
          onClick={() => setColorScheme('light')}
        >
          Light
        </Menu.Item>
        <Menu.Item
          leftSection={<IconDeviceDesktop size={16} />}
          rightSection={colorScheme === 'auto' ? <IconCheck size={14} aria-hidden /> : undefined}
          onClick={() => setColorScheme('auto')}
        >
          System
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
}
