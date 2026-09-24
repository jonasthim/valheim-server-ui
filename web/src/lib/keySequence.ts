// Pure prefix-key sequence state machine for the `g d` / `g j` / `g s` / `g
// a` navigation shortcuts (useGlobalShortcuts.ts), plus the target checks
// that keep the global shortcut layer out of the way of form controls and
// open dialogs. No React/DOM event types beyond EventTarget, so both stay
// trivial to reason about in isolation.

export const SEQUENCE_TIMEOUT_MS = 800

export interface SequenceState {
  prefix: string | null
  at: number
}

export const IDLE_SEQUENCE: SequenceState = { prefix: null, at: 0 }

/** Advances the sequence state machine by one keypress. `sequences` are
 * space-joined two-key combos (e.g. "g j"). A completed match resets to
 * idle and returns the matched sequence in `matched`; a key that could
 * start one of `sequences` becomes the new prefix; anything else resets to
 * idle with no match. */
export function stepSequence(
  state: SequenceState,
  key: string,
  now: number,
  sequences: readonly string[],
  timeoutMs = SEQUENCE_TIMEOUT_MS,
): { next: SequenceState; matched: string | null } {
  if (state.prefix && now - state.at <= timeoutMs) {
    const candidate = `${state.prefix} ${key}`
    if (sequences.includes(candidate)) {
      return { next: IDLE_SEQUENCE, matched: candidate }
    }
  }
  if (sequences.some((s) => s.startsWith(`${key} `))) {
    return { next: { prefix: key, at: now }, matched: null }
  }
  return { next: IDLE_SEQUENCE, matched: null }
}

/** True for form controls and contenteditable regions, where the shortcut
 * layer must not intercept keystrokes meant for typing. */
export function isEditableTarget(t: EventTarget | null): boolean {
  if (!(t instanceof HTMLElement)) return false
  return t.isContentEditable || t.tagName === 'INPUT' || t.tagName === 'TEXTAREA' || t.tagName === 'SELECT'
}

/** True when `t` sits inside an open dialog (the command palette, a
 * confirm modal, this card's own shortcuts help), so shortcuts don't fire
 * underneath it. */
export function isInsideDialog(t: EventTarget | null): boolean {
  return t instanceof HTMLElement && t.closest('[role="dialog"]') !== null
}
