import { Badge, Group, NavLink, ScrollArea, Stack, Text } from '@mantine/core'
import { Fragment } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { useAuth } from '../auth/useAuth'
import { stateColor, useInstances } from '../features/instances'
import { useSystemInfo } from '../features/system'
import { PAGES } from '../lib/routes'
import { BrandMark, StatusDot } from '../ui'
import { NAV_ICONS } from './navIcons'
import classes from './Shell.module.css'

const GROUPS = ['Servers', 'Administration'] as const

interface SidebarProps {
  onNavigate: () => void
}

// Sidebar nav: brand at the top, PAGES grouped by group with a dynamic
// "Instances" group injected after Servers, version + update badge in the
// footer. No user menu here any more (moved to the top bar's UserMenu).
export function Sidebar({ onNavigate }: SidebarProps) {
  const { hasRole } = useAuth()
  const loc = useLocation()
  const system = useSystemInfo()
  const instances = useInstances()

  const allInstances = instances.data ?? []
  const visibleInstances = allInstances.slice(0, 12)
  const hasMoreInstances = allInstances.length > 12
  const showInstancesGroup = allInstances.length > 0 && hasRole('viewer')

  return (
    <>
      <Link to="/" onClick={onNavigate} className={classes.brand}>
        <BrandMark size={22} />
        <span className={classes.wordmark}>Valheim Server UI</span>
      </Link>
      <ScrollArea style={{ flex: 1 }} px="xs" py="xs">
        {GROUPS.map((group) => {
          const items = PAGES.filter((p) => p.group === group && hasRole(p.min))
          return (
            <Fragment key={group}>
              {items.length > 0 && (
                <div>
                  <span className={classes.groupLabel}>{group}</span>
                  <Stack gap={2}>
                    {items.map((p) => {
                      const Icon = NAV_ICONS[p.to]
                      const active = p.to === '/' ? loc.pathname === '/' : loc.pathname.startsWith(p.to)
                      return (
                        <NavLink
                          key={p.to}
                          component={Link}
                          to={p.to}
                          label={p.label}
                          className={classes.link}
                          leftSection={
                            <span className={classes.linkIcon}>
                              <Icon size={18} stroke={1.8} />
                            </span>
                          }
                          active={active}
                          variant="subtle"
                          onClick={onNavigate}
                        />
                      )
                    })}
                  </Stack>
                </div>
              )}
              {group === 'Servers' && showInstancesGroup && (
                <div>
                  <span className={classes.groupLabel}>Instances</span>
                  <Stack gap={2}>
                    {visibleInstances.map((instance) => {
                      const active = loc.pathname.startsWith(`/instances/${instance.id}/`)
                      return (
                        <NavLink
                          key={instance.id}
                          component={Link}
                          to={`/instances/${instance.id}/overview`}
                          label={instance.name}
                          className={classes.link}
                          leftSection={
                            <StatusDot color={stateColor(instance.status.state)} pulse={instance.status.state === 'running'} />
                          }
                          rightSection={
                            instance.status.players_online > 0 ? (
                              <Text size="xs" c="dimmed">
                                {instance.status.players_online}
                              </Text>
                            ) : undefined
                          }
                          active={active}
                          variant="subtle"
                          onClick={onNavigate}
                        />
                      )
                    })}
                    {hasMoreInstances && (
                      <NavLink
                        component={Link}
                        to="/"
                        label="All instances"
                        className={classes.link}
                        variant="subtle"
                        onClick={onNavigate}
                      />
                    )}
                  </Stack>
                </div>
              )}
            </Fragment>
          )
        })}
      </ScrollArea>
      <div className={classes.navFooter}>
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
    </>
  )
}
