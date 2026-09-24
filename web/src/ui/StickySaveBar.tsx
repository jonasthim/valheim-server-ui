// Fixed footer for a page-owned form (the C-4 seam), paired with
// useUnsavedChanges (B-6). Lives outside the <form> it submits; the Save
// button's `form` attribute associates it.
import { Button, Group, Text, Transition } from '@mantine/core'
import { IconAlertCircle } from '@tabler/icons-react'
import classes from './StickySaveBar.module.css'

export interface StickySaveBarProps {
  formId: string
  show: boolean
  dirty: boolean
  saveLabel: string
  saving?: boolean
  saveDisabled?: boolean
  onDiscard: () => void
}

export function StickySaveBar({ formId, show, dirty, saveLabel, saving, saveDisabled, onDiscard }: StickySaveBarProps) {
  return (
    <>
      {/* Reserves the bar's height so the last field is never hidden under it. */}
      <div className={classes.spacer} data-visible={show || undefined} />
      <Transition mounted={show} transition="slide-up" duration={160}>
        {(styles) => (
          <div className={classes.bar} role="region" aria-label="Unsaved changes" style={styles}>
            <Group gap="xs">
              {dirty && (
                <>
                  <IconAlertCircle size={16} />
                  <Text size="sm" fw={500}>
                    Unsaved changes
                  </Text>
                </>
              )}
            </Group>
            <Group gap="xs">
              {dirty && (
                <Button variant="default" onClick={onDiscard}>
                  Discard
                </Button>
              )}
              <Button type="submit" form={formId} loading={saving} disabled={saveDisabled}>
                {saveLabel}
              </Button>
            </Group>
          </div>
        )}
      </Transition>
    </>
  )
}
