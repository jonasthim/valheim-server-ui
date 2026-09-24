import { ActionIcon, Box, Burger, Button, Group, Kbd } from '@mantine/core'
import { useOs } from '@mantine/hooks'
import { IconSearch } from '@tabler/icons-react'
import { Link } from 'react-router-dom'
import { ActivityIndicator } from '../features/jobs/ActivityIndicator'
import { LiveStatusBadge } from '../features/system'
import { BrandMark } from '../ui'
import { RouteBreadcrumbs } from './RouteBreadcrumbs'
import { UserMenu } from './UserMenu'

interface TopBarProps {
  navOpened: boolean
  onToggleNav: () => void
  onOpenPalette?: () => void
}

// 48px product top bar: burger (mobile) + route breadcrumbs on the left,
// command palette / activity / live status / account menu on the right. The
// palette button only renders once a handler is wired up (Shell passes none
// yet, so it stays absent until the palette itself lands).
export function TopBar({ navOpened, onToggleNav, onOpenPalette }: TopBarProps) {
  const os = useOs()
  const modLabel = os === 'macos' || os === 'ios' ? '⌘' : 'Ctrl '

  return (
    <Group h="100%" px="md" justify="space-between" wrap="nowrap">
      <Group gap="sm" wrap="nowrap" style={{ minWidth: 0 }}>
        <Burger opened={navOpened} onClick={onToggleNav} hiddenFrom="sm" size="sm" aria-label="Toggle navigation" />
        <Box component={Link} to="/" hiddenFrom="sm" aria-label="Valheim Server UI">
          <BrandMark size={22} />
        </Box>
        <RouteBreadcrumbs />
      </Group>
      <Group gap="xs" wrap="nowrap">
        {onOpenPalette && (
          <>
            <Button
              variant="default"
              size="sm"
              visibleFrom="sm"
              aria-label="Search commands"
              leftSection={<IconSearch size={16} />}
              rightSection={
                <Kbd size="xs" aria-hidden>
                  {modLabel}K
                </Kbd>
              }
              onClick={onOpenPalette}
            >
              Search
            </Button>
            <ActionIcon variant="subtle" color="gray" size="lg" hiddenFrom="sm" aria-label="Search commands" onClick={onOpenPalette}>
              <IconSearch size={16} />
            </ActionIcon>
          </>
        )}
        <ActivityIndicator />
        <LiveStatusBadge />
        <UserMenu />
      </Group>
    </Group>
  )
}
