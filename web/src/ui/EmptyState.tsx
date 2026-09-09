import type { ReactNode } from 'react'
import { Text } from '@mantine/core'
import classes from './ui.module.css'

/** Dashed empty-state panel with an icon tile, title, hint and optional action. */
export function EmptyState({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
}) {
  return (
    <div className={classes.emptyState}>
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
