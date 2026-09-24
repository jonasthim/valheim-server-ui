// Interim save control living outside the form it submits, associated via
// the `form` attribute. B-6 replaces every call site with StickySaveBar.
import { Button, Group } from '@mantine/core'
import classes from './FormFooter.module.css'

export function FormFooter({
  formId,
  submitLabel,
  submitting,
}: {
  formId: string
  submitLabel: string
  submitting?: boolean
}) {
  return (
    <Group justify="flex-end" className={classes.footer}>
      <Button type="submit" form={formId} loading={submitting}>
        {submitLabel}
      </Button>
    </Group>
  )
}
