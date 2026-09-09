import type { ReactNode } from 'react'
import { Group, Stack, Text, Title } from '@mantine/core'
import classes from './ui.module.css'

/**
 * Page title row: eyebrow (optional), title, description and an actions slot.
 * Use once at the top of every page for consistent rhythm.
 */
export function PageHeader({
  eyebrow,
  title,
  description,
  actions,
  titleAddon,
}: {
  eyebrow?: ReactNode
  title: ReactNode
  description?: ReactNode
  actions?: ReactNode
  /** Rendered inline after the title (e.g. a StatusPill). */
  titleAddon?: ReactNode
}) {
  return (
    <div className={classes.pageHeader}>
      <Stack gap={4} style={{ minWidth: 0 }}>
        {eyebrow && <span className="vh-eyebrow">{eyebrow}</span>}
        <Group gap="sm" wrap="nowrap" align="center">
          <Title order={1} className={classes.pageTitle} size="h2" style={{ minWidth: 0 }} lineClamp={1}>
            {title}
          </Title>
          {titleAddon}
        </Group>
        {description && (
          <Text size="sm" c="dimmed">
            {description}
          </Text>
        )}
      </Stack>
      {actions && (
        <Group gap="xs" wrap="wrap">
          {actions}
        </Group>
      )}
    </div>
  )
}
