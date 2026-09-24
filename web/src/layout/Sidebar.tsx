import { ActionIcon, Badge, Group, Indicator, NavLink, ScrollArea, Stack, Text, Tooltip, UnstyledButton } from '@mantine/core'
import { Fragment } from 'react'
import { Link, useLocation } from 'react-router-dom'
import { IconLayoutSidebarLeftCollapse, IconLayoutSidebarLeftExpand, IconList } from '@tabler/icons-react'
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
  rail: boolean
  collapsed: boolean
  onToggleCollapsed: () => void
}

// Sidebar nav: brand at the top, PAGES grouped by group with a dynamic
// "Instances" group injected after Servers, version + update badge in the
// footer. No user menu here any more (moved to the top bar's UserMenu).
// `rail` (desktop-only, collapsed) swaps grouped NavLinks for a 56px icon
// rail with tooltips; group labels become hairline dividers.
export function Sidebar({ onNavigate, rail, collapsed, onToggleCollapsed }: SidebarProps) {
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
        {!rail && <span className={classes.wordmark}>Valheim Server UI</span>}
      </Link>
      <ScrollArea style={{ flex: 1 }} px="xs" py="xs">
        {GROUPS.map((group) => {
          const items = PAGES.filter((p) => p.group === group && hasRole(p.min))
          return (
            <Fragment key={group}>
              {items.length > 0 && (
                <div>
                  {rail ? (
                    <div className={classes.railDivider} />
                  ) : (
                    <span className={classes.groupLabel}>{group}</span>
                  )}
                  <Stack gap={2}>
                    {items.map((p) => {
                      const Icon = NAV_ICONS[p.to]
                      const active = p.to === '/' ? loc.pathname === '/' : loc.pathname.startsWith(p.to)
                      if (rail) {
                        return (
                          <Tooltip key={p.to} label={p.label} position="right">
                            <UnstyledButton
                              component={Link}
                              to={p.to}
                              className={classes.railLink}
                              aria-label={p.label}
                              aria-current={active ? 'page' : undefined}
                              data-active={active || undefined}
                              onClick={onNavigate}
                            >
                              <Icon size={18} stroke={1.8} />
                            </UnstyledButton>
                          </Tooltip>
                        )
                      }
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
                  {rail ? (
                    <div className={classes.railDivider} />
                  ) : (
                    <span className={classes.groupLabel}>Instances</span>
                  )}
                  <Stack gap={2}>
                    {visibleInstances.map((instance) => {
                      const active = loc.pathname.startsWith(`/instances/${instance.id}/`)
                      const count = instance.status.players_online
                      if (rail) {
                        return (
                          <Tooltip
                            key={instance.id}
                            label={count > 0 ? `${instance.name} (${count} online)` : instance.name}
                            position="right"
                          >
                            <UnstyledButton
                              component={Link}
                              to={`/instances/${instance.id}/overview`}
                              className={classes.railLink}
                              aria-label={instance.name}
                              aria-current={active ? 'page' : undefined}
                              data-active={active || undefined}
                              onClick={onNavigate}
                            >
                              <Indicator label={count} size={14} color="frost" disabled={count === 0}>
                                <StatusDot color={stateColor(instance.status.state)} pulse={instance.status.state === 'running'} />
                              </Indicator>
                            </UnstyledButton>
                          </Tooltip>
                        )
                      }
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
                      rail ? (
                        <Tooltip label="All instances" position="right">
                          <UnstyledButton
                            component={Link}
                            to="/"
                            className={classes.railLink}
                            aria-label="All instances"
                            onClick={onNavigate}
                          >
                            <IconList size={18} stroke={1.8} />
                          </UnstyledButton>
                        </Tooltip>
                      ) : (
                        <NavLink
                          component={Link}
                          to="/"
                          label="All instances"
                          className={classes.link}
                          variant="subtle"
                          onClick={onNavigate}
                        />
                      )
                    )}
                  </Stack>
                </div>
              )}
            </Fragment>
          )
        })}
      </ScrollArea>
      <div className={classes.navFooter}>
        <Group justify="space-between" px={8} pt={6} wrap="nowrap" gap={4}>
          {!rail && (
            <Text size="xs" c="dimmed">
              {system.data?.version ? `v${system.data.version.replace(/^v/, '')}` : ''}
            </Text>
          )}
          {system.data?.app_update?.update_available && (
            rail ? (
              <Tooltip label="Update available" position="right">
                <Badge
                  component={Link}
                  to="/settings"
                  circle
                  w={8}
                  h={8}
                  p={0}
                  color="ember"
                  variant="filled"
                  aria-label="Update available"
                  style={{ cursor: 'pointer' }}
                />
              </Tooltip>
            ) : (
              <Badge size="xs" color="ember" variant="filled" component={Link} to="/settings" style={{ cursor: 'pointer' }}>
                Update
              </Badge>
            )
          )}
          <Tooltip label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'} position="right">
            <ActionIcon
              variant="subtle"
              color="gray"
              size="sm"
              visibleFrom="sm"
              aria-label={collapsed ? 'Expand sidebar' : 'Collapse sidebar'}
              onClick={onToggleCollapsed}
            >
              {collapsed ? (
                <IconLayoutSidebarLeftExpand size={16} stroke={1.8} />
              ) : (
                <IconLayoutSidebarLeftCollapse size={16} stroke={1.8} />
              )}
            </ActionIcon>
          </Tooltip>
        </Group>
      </div>
    </>
  )
}
