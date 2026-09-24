import type { ReactNode } from 'react'
import { Button, Group } from '@mantine/core'
import { IconX } from '@tabler/icons-react'
import classes from './Toolbar.module.css'

export const TOOLBAR_INPUT_WIDTH = 240

/**
 * Hairline filter row for list-page `SectionCard flush` bodies: filter
 * controls on the left, an optional "Clear filters" button on the right that
 * only renders when `canClear` is true.
 */
export function Toolbar({
  children,
  canClear,
  onClear,
}: {
  children: ReactNode
  canClear?: boolean
  onClear?: () => void
}) {
  return (
    <div className={classes.toolbar}>
      <Group gap="sm" wrap="wrap" style={{ flex: 1 }}>
        {children}
      </Group>
      {canClear && (
        <Button variant="subtle" color="gray" size="sm" leftSection={<IconX size={14} />} onClick={onClear}>
          Clear filters
        </Button>
      )}
    </div>
  )
}
