import { AppShell, Center, Loader } from '@mantine/core'
import { Suspense } from 'react'
import { useDisclosure } from '@mantine/hooks'
import { Outlet, useLocation } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import { useEvents } from '../events/useEvents'
import { JobDrawerHost } from '../features/jobs/JobDrawerHost'
import { UpgradeFlowHost } from '../features/system'
import { ErrorBoundary } from '../ui'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import classes from './Shell.module.css'

export function Shell() {
  const [opened, { toggle, close }] = useDisclosure()
  const { user } = useAuth()
  const loc = useLocation()
  useEvents(!!user)

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
          navbar={{ width: 250, breakpoint: 'sm', collapsed: { mobile: !opened } }}
          padding={{ base: 'md', md: 'xl' }}
        >
          <AppShell.Header className={classes.header}>
            <TopBar navOpened={opened} onToggleNav={toggle} />
          </AppShell.Header>

          <AppShell.Navbar className={classes.navbar}>
            <Sidebar onNavigate={close} />
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
    </JobDrawerHost>
  )
}
