import { AppShell, Center, Loader, useMantineTheme } from '@mantine/core'
import { Suspense } from 'react'
import { useDisclosure, useHotkeys, useLocalStorage, useMediaQuery } from '@mantine/hooks'
import { Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import { useEvents } from '../events/useEvents'
import { JobDrawerHost } from '../features/jobs/JobDrawerHost'
import { CommandPalette } from '../features/palette/CommandPalette'
import { openPalette } from '../features/palette/paletteStore'
import { UpgradeFlowHost } from '../features/system'
import { ErrorBoundary } from '../ui'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import classes from './Shell.module.css'

export function Shell() {
  const [opened, { toggle, close }] = useDisclosure()
  const [collapsed, setCollapsed] = useLocalStorage<boolean>({
    key: 'vh-sidebar-collapsed',
    defaultValue: false,
    getInitialValueInEffect: false,
  })
  const { user } = useAuth()
  const loc = useLocation()
  const theme = useMantineTheme()
  const isDesktop = useMediaQuery(`(min-width: ${theme.breakpoints.sm})`, true, { getInitialValueInEffect: false })
  const rail = collapsed && isDesktop
  useEvents(!!user)
  // Temporary: B-4 moves both into the keyboard-shortcuts layer.
  useHotkeys([['mod+B', () => setCollapsed((c) => !c)]])
  useHotkeys([['mod+K', openPalette]], [])

  return (
    <JobDrawerHost>
      {/* WCAG 2.4.1: lets keyboard users jump past the header/nav straight
          to the routed page. Hidden until focused (Shell.module.css). */}
      <a href="#main-content" className={classes.skipLink}>
        Skip to content
      </a>
      <UpgradeFlowHost>
        <AppShell
          header={{ height: 48 }}
          navbar={{ width: rail ? 56 : 232, breakpoint: 'sm', collapsed: { mobile: !opened, desktop: false } }}
          padding={{ base: 'md', md: 'xl' }}
          transitionDuration={160}
        >
          <AppShell.Header className={classes.header}>
            <TopBar navOpened={opened} onToggleNav={toggle} onOpenPalette={openPalette} />
          </AppShell.Header>

          <AppShell.Navbar className={classes.navbar}>
            <Sidebar onNavigate={close} rail={rail} collapsed={collapsed} onToggleCollapsed={() => setCollapsed((c) => !c)} />
          </AppShell.Navbar>

          <AppShell.Main className={classes.main} id="main-content" tabIndex={-1}>
            <div className={classes.content}>
              <ErrorBoundary key={loc.pathname}>
                <Suspense fallback={<Center h="50vh"><Loader /></Center>}>
                  <Outlet />
                </Suspense>
              </ErrorBoundary>
            </div>
          </AppShell.Main>
        </AppShell>
      </UpgradeFlowHost>
      <CommandPalette />
    </JobDrawerHost>
  )
}
