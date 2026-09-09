// Small standard 5-field cron helpers: presets for the schedule editor, a
// human-readable description for common forms, and client-side shape
// validation. Not a general cron parser — Valheim schedules only need a
// handful of shapes (fixed time, step, weekly), everything else falls back
// to showing the raw expression.

export const CUSTOM_CRON_PRESET = 'custom'

export const CRON_PRESETS: { label: string; value: string }[] = [
  { label: 'Daily at 04:00', value: '0 4 * * *' },
  { label: 'Every 6 hours', value: '0 */6 * * *' },
  { label: 'Every hour', value: '0 * * * *' },
  { label: 'Weekly Sunday 05:00', value: '0 5 * * 0' },
  { label: 'Custom', value: CUSTOM_CRON_PRESET },
]

const DAY_NAMES = ['Sunday', 'Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday']

const FIELD_PATTERN = /^(\*|\*\/\d+|\d+(-\d+)?(,\d+(-\d+)?)*)$/

/** Returns an error message if `expr` is not a plausible 5-field cron expression, else null. */
export function validateCronExpr(expr: string): string | null {
  const parts = expr.trim().split(/\s+/)
  if (parts.length !== 5) {
    return 'Must have 5 space-separated fields: minute hour day-of-month month day-of-week'
  }
  if (!parts.every((p) => FIELD_PATTERN.test(p))) {
    return 'Each field must be *, a number, a step (*/n) or a comma/range list'
  }
  return null
}

function pad(n: string): string {
  return n.padStart(2, '0')
}

function parseDow(field: string): string | null {
  const nums = field.split(',').map((n) => Number(n))
  if (nums.some((n) => Number.isNaN(n) || n < 0 || n > 6)) return null
  const names = nums.map((n) => DAY_NAMES[n])
  return names.join(', ')
}

/** Best-effort human description of a standard 5-field cron expression. */
export function cronDescribe(expr: string): string {
  const trimmed = expr.trim()
  if (validateCronExpr(trimmed)) return trimmed

  const [min, hour, dom, month, dow] = trimmed.split(/\s+/)
  if (dom !== '*' || month !== '*') return trimmed

  if (min === '*' && hour === '*' && dow === '*') return 'Every minute'

  const minStep = /^\*\/(\d+)$/.exec(min)
  if (minStep && hour === '*' && dow === '*') {
    const n = minStep[1]
    return `Every ${n} minute${n === '1' ? '' : 's'}`
  }

  const isNum = (s: string) => /^\d+$/.test(s)

  if (isNum(min) && hour === '*' && dow === '*') {
    return min === '0' ? 'Every hour' : `Every hour at :${pad(min)}`
  }

  const hourStep = /^\*\/(\d+)$/.exec(hour)
  if (isNum(min) && hourStep && dow === '*') {
    const n = hourStep[1]
    const suffix = min === '0' ? '' : ` (at :${pad(min)})`
    return `Every ${n} hour${n === '1' ? '' : 's'}${suffix}`
  }

  if (isNum(min) && isNum(hour) && dow === '*') {
    return `Daily at ${pad(hour)}:${pad(min)}`
  }

  if (isNum(min) && isNum(hour) && dow !== '*') {
    const days = parseDow(dow)
    if (days) return `Weekly on ${days} at ${pad(hour)}:${pad(min)}`
  }

  return trimmed
}

/** Which preset (if any) matches an expression exactly, for pre-selecting the Select. */
export function presetForExpr(expr: string): string {
  const match = CRON_PRESETS.find((p) => p.value === expr)
  return match ? match.value : CUSTOM_CRON_PRESET
}
