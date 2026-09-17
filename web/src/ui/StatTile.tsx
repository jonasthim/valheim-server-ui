import type { CSSProperties, ReactNode } from 'react'
import { Group, Paper, Text } from '@mantine/core'
import classes from './ui.module.css'

/**
 * KPI tile: eyebrow label, big value, optional hint line and icon.
 * `accent` is any CSS colour (use the --vh-* tokens) drawn as a left bar.
 * `compact` shrinks vertical padding and the value's font size (inline
 * style only, so it composes with any background/color the caller applies).
 */
export function StatTile({
  label,
  value,
  hint,
  icon,
  accent,
  compact = false,
}: {
  label: ReactNode
  value: ReactNode
  hint?: ReactNode
  icon?: ReactNode
  accent?: string
  compact?: boolean
}) {
  const rootStyle = {
    ...(accent ? { '--tile-accent': accent } : {}),
    ...(compact ? { padding: '8px 12px', minHeight: 'unset', gap: 4 } : {}),
  } as CSSProperties
  return (
    <Paper
      className={classes.statTile}
      data-accent={accent ? '' : undefined}
      style={Object.keys(rootStyle).length > 0 ? rootStyle : undefined}
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
        <div
          className={classes.statValue}
          data-long={typeof value === 'string' && value.length > 12 ? '' : undefined}
          style={compact ? { fontSize: '1.1rem' } : undefined}
        >
          {value}
        </div>
        {hint && (
          <Text size="xs" c="dimmed" mt={2}>
            {hint}
          </Text>
        )}
      </div>
    </Paper>
  )
}
