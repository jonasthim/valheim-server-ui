import type { ReactNode } from 'react'
import { Text } from '@mantine/core'
import classes from './ui.module.css'

/**
 * Empty-state panel with an icon tile, title, hint and optional action.
 * `compact` tightens the padding.
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
  compact,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  compact?: boolean
}) {
  const rootClassName = [classes.emptyState, compact && classes.emptyCompact].filter(Boolean).join(' ')

  return (
    <div className={rootClassName}>
      {icon && <div className={classes.emptyIcon}>{icon}</div>}
      <Text fw={600}>{title}</Text>
      {description && (
        <Text size="sm" c="dimmed" maw={420}>
          {description}
        </Text>
      )}
      {action}
    </div>
  )
}
