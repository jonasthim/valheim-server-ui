import type { ReactNode } from 'react'
import { Group, Stack, Text, Title } from '@mantine/core'
import { BrandMark } from '../../ui'
import classes from './Auth.module.css'

/**
 * Centred shell for /login and /setup: one timber panel with the brand
 * header on a parchment backdrop.
 */
export function AuthLayout({ intro, children }: { intro?: string; children: ReactNode }) {
  return (
    <div className={classes.page}>
      <div className={classes.panel}>
        <Stack gap="lg">
          <Group gap="sm" wrap="nowrap">
            <BrandMark size={32} />
            <Title order={1}>Valheim Server UI</Title>
          </Group>
          {intro && (
            <Text size="sm" c="dimmed">
              {intro}
            </Text>
          )}
          {children}
        </Stack>
      </div>
    </div>
  )
}
