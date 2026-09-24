// Shared unsaved-changes guard for every page-owned @mantine/form (C-4's
// seam): blocks in-app navigation with a "Discard changes?" confirm, warns on
// tab close/reload, and exposes `dirty` for StickySaveBar. Hooks only, no
// JSX — see web/src/ui/StickySaveBar.tsx for the bar itself.
import { useCallback, useEffect, useRef } from 'react'
import { useBlocker, type BlockerFunction } from 'react-router-dom'
import { modals } from '@mantine/modals'
import type { UseFormReturnType } from '@mantine/form'

/** Blocks in-app navigation (and warns on tab close/reload) while `dirty` is
 * true, confirming with a "Discard changes?" modal before letting the user
 * leave. Never blocks navigation to `/login` (e.g. a session expiring). */
export function useLeaveGuard(dirty: boolean): void {
  const dirtyRef = useRef(dirty)
  // "Latest ref" pattern: shouldBlock/beforeunload run outside React's render
  // cycle (router/browser callbacks), so they need the current value without
  // becoming a dependency that would tear down and rebuild the blocker.
  // eslint-disable-next-line react/refs
  dirtyRef.current = dirty

  const shouldBlock = useCallback<BlockerFunction>(
    ({ currentLocation, nextLocation }) =>
      dirtyRef.current && currentLocation.pathname !== nextLocation.pathname && nextLocation.pathname !== '/login',
    [],
  )
  const blocker = useBlocker(shouldBlock)

  useEffect(() => {
    if (blocker.state !== 'blocked') return
    if (!dirtyRef.current) {
      blocker.proceed()
      return
    }
    let decided = false
    modals.openConfirmModal({
      modalId: 'unsaved-changes',
      title: 'Discard changes?',
      centered: true,
      children: 'You have unsaved changes. Leave this page and lose them?',
      labels: { confirm: 'Discard', cancel: 'Keep editing' },
      confirmProps: { color: 'red' },
      onConfirm: () => {
        decided = true
        blocker.proceed()
      },
      onCancel: () => {
        decided = true
        blocker.reset()
      },
      onClose: () => {
        if (!decided) blocker.reset()
      },
    })
    return () => {
      modals.close('unsaved-changes')
    }
  }, [blocker])

  useEffect(() => {
    if (!dirty) return
    function handler(e: BeforeUnloadEvent) {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [dirty])
}

export interface UnsavedChanges {
  dirty: boolean
  /** Moves the form's dirty baseline to its current values; call before a
   * successful save's navigation/notification so the leave guard disarms. */
  markClean: () => void
  /** Reverts the form to its last clean snapshot. */
  discard: () => void
}

/** `useLeaveGuard` driven by a `@mantine/form` instance's own dirty tracking. */
export function useUnsavedChanges<T>(
  form: UseFormReturnType<T>,
  opts: { enabled?: boolean } = {},
): UnsavedChanges {
  const dirty = (opts.enabled ?? true) && form.isDirty()
  useLeaveGuard(dirty)
  return {
    dirty,
    markClean: () => form.resetDirty(),
    // Not form.reset(): that restores the useForm initialValues (an empty
    // shape on pages that seed from a query), while the dirty snapshot set by
    // initialize()/resetDirty() is the last saved state we want back.
    discard: () => {
      form.setValues(structuredClone(form.getInitialValues()))
      form.clearErrors()
      form.resetDirty()
    },
  }
}
