import type { ReactNode } from 'react'
import { Group, Paper, Text } from '@mantine/core'
import classes from './ui.module.css'

/**
 * KPI tile: eyebrow label, big value, optional hint line and icon.
 * `accent` is any CSS colour (use the --vh-* tokens) drawn as a left bar.
 */
export function StatTile({
  label,
  value,
  hint,
  icon,
  accent,
}: {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
  icon?: ReactNode
  accent?: string
}) {
  return (
    <Paper
      className={classes.statTile}
      data-accent={accent ? '' : undefined}
      style={accent ? ({ '--tile-accent': accent } as React.CSSProperties) : undefined}
    >
      <Group justify="space-between" wrap="nowrap" align="flex-start">
        <span className="vh-eyebrow">{label}</span>
        {icon && (
          <Text c="dimmed" component="span" style={{ display: 'inline-flex' }}>
            {icon}
          </Text>
        )}
      </Group>
      <div>
        <div className={classes.statValue}>{value}</div>
        {hint && (
          <Text size="xs" c="dimmed" mt={2}>
            {hint}
          </Text>
        )}
      </div>
    </Paper>
  )
}
