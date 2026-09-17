import type { ReactNode } from 'react'
import { Text, Title } from '@mantine/core'
import classes from './ui.module.css'

/**
 * Empty-state panel with an icon tile, title, hint and optional action.
 * `variant="parchment"` renders it as an ink-on-paper invitation to act
 * instead of the default dashed grey panel; `compact` tightens the padding.
 */
export function EmptyState({
  icon,
  title,
  description,
  action,
  variant = 'default',
  compact,
}: {
  icon?: ReactNode
  title: ReactNode
  description?: ReactNode
  action?: ReactNode
  variant?: 'default' | 'parchment'
  compact?: boolean
}) {
  const rootClassName = [
    classes.emptyState,
    variant === 'parchment' && classes.emptyParchment,
    compact && classes.emptyCompact,
  ]
    .filter(Boolean)
    .join(' ')

  return (
    <div className={rootClassName}>
      {icon && <div className={classes.emptyIcon}>{icon}</div>}
      {variant === 'parchment' ? (
        <Title order={3} ff="var(--mantine-heading-font-family)" c="var(--vh-ink)">
          {title}
        </Title>
      ) : (
        <Text fw={600}>{title}</Text>
      )}
      {description && (
        <Text size="sm" c="dimmed" maw={420}>
          {description}
        </Text>
      )}
      {action}
    </div>
  )
}
