import { Anchor, Box, Breadcrumbs, Text } from '@mantine/core'
import { IconChevronRight } from '@tabler/icons-react'
import { Link, useLocation } from 'react-router-dom'
import { useInstances } from '../features/instances'
import { deriveBreadcrumbs } from '../lib/routes'

// Route-derived breadcrumb trail in the top bar (TopBar.tsx). Renders nothing
// on pages with no trail (an exact PAGES match collapses to a single current
// label instead, per deriveBreadcrumbs).
export function RouteBreadcrumbs() {
  const { pathname } = useLocation()
  const instances = useInstances()
  const crumbs = deriveBreadcrumbs(pathname, (id) => instances.data?.find((i) => i.id === id)?.name)

  if (crumbs.length === 0) return null

  const last = crumbs.length - 1

  return (
    <nav aria-label="Breadcrumb">
      <Box visibleFrom="sm">
        <Breadcrumbs separator={<IconChevronRight size={14} aria-hidden />} separatorMargin={6}>
          {crumbs.map((c, i) =>
            i === last || !c.to ? (
              <Text key={i} size="sm" fw={600} aria-current="page">
                {c.label}
              </Text>
            ) : (
              <Anchor key={i} component={Link} to={c.to} size="sm" c="dimmed" underline="never">
                {c.label}
              </Anchor>
            ),
          )}
        </Breadcrumbs>
      </Box>
      <Box hiddenFrom="sm">
        <Text size="sm" fw={600} aria-current="page">
          {crumbs[last].label}
        </Text>
      </Box>
    </nav>
  )
}
