export const APP_NAME = 'Valheim Server UI'

/** Non-empty, trimmed parts followed by APP_NAME, joined with ' · '. */
export function pageTitle(...parts: Array<string | undefined>): string {
  const segments = parts.map((p) => p?.trim()).filter((p): p is string => !!p)
  return [...segments, APP_NAME].join(' · ')
}
