import type { ReactNode } from 'react'
import { Paper, Stack, Text, Title } from '@mantine/core'
import classes from './ui.module.css'

/**
 * Card with a titled header row (title, description, actions) and a body.
 * `flush` removes body padding for tables that should run edge to edge.
 */
export function SectionCard({
  title,
  description,
  actions,
  children,
  flush = false,
  className,
}: {
  title?: ReactNode
  description?: ReactNode
  actions?: ReactNode
  children: ReactNode
  flush?: boolean
  className?: string
}) {
  return (
    <Paper className={[classes.sectionCard, className].filter(Boolean).join(' ')}>
      {(title || actions) && (
        <div className={classes.sectionHead}>
          <Stack gap={2} style={{ minWidth: 0 }}>
            {title && (
              <Title order={3} size="h4">
                {title}
              </Title>
            )}
            {description && (
              <Text size="sm" c="dimmed">
                {description}
              </Text>
            )}
          </Stack>
          {actions && <div style={{ flex: 'none' }}>{actions}</div>}
        </div>
      )}
      <div className={flush ? classes.sectionBodyFlush : classes.sectionBody}>{children}</div>
    </Paper>
  )
}
