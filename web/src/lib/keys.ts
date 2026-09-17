let seq = 0

/**
 * Stable React keys for locally-created rows; crypto.randomUUID is
 * unavailable on plain-http (non-secure) origins, which a LAN deployment is.
 */
export function newKey(prefix = 'k'): string {
  seq += 1
  return `${prefix}-${Date.now().toString(36)}-${seq.toString(36)}`
}
