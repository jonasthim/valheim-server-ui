// Query/mutation hooks for the Console tab's log file picker, static file
// viewer and cross-file search (F-2.6).
import { useMutation, useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { LogFileInfo, LogMatch } from '../../api/types'

/** GET /instances/{id}/logs/files, refreshed every 30s so a newly-rotated log shows up. */
export function useLogFiles(id: string) {
  return useQuery({
    queryKey: ['instances', id, 'logs', 'files'],
    queryFn: () => api.get<{ files: LogFileInfo[] }>(`/instances/${id}/logs/files`).then((r) => r.files),
    enabled: !!id,
    refetchInterval: 30_000,
  })
}

/** GET /instances/{id}/logs/files/{name}, the tail of one non-live log file. */
export function useLogFileTail(id: string, name: string, enabled: boolean) {
  return useQuery({
    queryKey: ['instances', id, 'logs', 'file', name],
    queryFn: () =>
      api.get<{ lines: string[] }>(`/instances/${id}/logs/files/${encodeURIComponent(name)}`).then((r) => r.lines),
    enabled: enabled && !!id && !!name,
  })
}

/** GET /instances/{id}/logs/search, run on demand (not auto-fetched). */
export function useLogSearch(id: string) {
  return useMutation({
    mutationFn: ({ q, regex }: { q: string; regex: boolean }) =>
      api.get<{ matches: LogMatch[] }>(`/instances/${id}/logs/search`, { q, regex }).then((r) => r.matches),
  })
}
