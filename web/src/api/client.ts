// Thin fetch wrapper for /api/v1. Adds the CSRF header, parses the documented
// error body into ApiError, and exposes typed helpers.
import type { ErrorResponse } from './types'

export const API_BASE = '/api/v1'
export const CSRF_HEADER = 'X-Requested-With'
export const CSRF_VALUE = 'valheim-ui'

export class ApiError extends Error {
  status: number
  code: string
  details?: Record<string, unknown>
  fields: { field: string; message: string }[]
  constructor(status: number, body?: ErrorResponse) {
    super(body?.error?.message ?? `HTTP ${status}`)
    this.status = status
    this.code = body?.error?.code ?? 'http_error'
    this.details = body?.error?.details
    this.fields = (body?.error?.details?.fields as { field: string; message: string }[] | undefined) ?? []
  }
  /** Map field errors to a Mantine form `setErrors` payload. */
  fieldErrors(): Record<string, string> {
    return Object.fromEntries(this.fields.map((f) => [f.field, f.message]))
  }
}

type Query = Record<string, string | number | boolean | undefined | null>

function qs(query?: Query): string {
  if (!query) return ''
  const p = new URLSearchParams()
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === null || v === '') continue
    p.set(k, String(v))
  }
  const s = p.toString()
  return s ? `?${s}` : ''
}

async function parseError(res: Response): Promise<ApiError> {
  let body: ErrorResponse | undefined
  try {
    body = (await res.json()) as ErrorResponse
  } catch {
    body = undefined
  }
  return new ApiError(res.status, body)
}

export async function request<T>(
  method: string,
  path: string,
  opts: { body?: unknown; query?: Query; form?: FormData; signal?: AbortSignal } = {},
): Promise<T> {
  const headers: Record<string, string> = { Accept: 'application/json' }
  if (method !== 'GET' && method !== 'HEAD') headers[CSRF_HEADER] = CSRF_VALUE
  let body: BodyInit | undefined
  if (opts.form) {
    body = opts.form
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  const res = await fetch(`${API_BASE}${path}${qs(opts.query)}`, {
    method,
    headers,
    body,
    credentials: 'same-origin',
    signal: opts.signal,
  })
  if (!res.ok) throw await parseError(res)
  if (res.status === 204) return undefined as T
  const ct = res.headers.get('content-type') ?? ''
  if (ct.includes('application/json')) return (await res.json()) as T
  return (await res.text()) as unknown as T
}

export const api = {
  get: <T>(path: string, query?: Query, signal?: AbortSignal) => request<T>('GET', path, { query, signal }),
  post: <T>(path: string, body?: unknown, query?: Query) => request<T>('POST', path, { body, query }),
  put: <T>(path: string, body?: unknown) => request<T>('PUT', path, { body }),
  patch: <T>(path: string, body?: unknown) => request<T>('PATCH', path, { body }),
  del: <T>(path: string, query?: Query) => request<T>('DELETE', path, { query }),
  upload: <T>(path: string, form: FormData) => request<T>('POST', path, { form }),
  /** Absolute URL for download links (browser navigation, not fetch). */
  url: (path: string) => `${API_BASE}${path}`,
}
