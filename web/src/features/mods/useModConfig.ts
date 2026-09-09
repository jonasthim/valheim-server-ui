// Query/mutation hooks for the BepInEx config file editor.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { ConfigFile, ConfigFileInfo, ConfigFileUpdate } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

/** GET /instances/{id}/mods/configs */
export function useConfigFiles(id: string) {
  return useQuery({
    queryKey: ['instances', id, 'mods', 'configs'],
    queryFn: () => api.get<{ files: ConfigFileInfo[] }>(`/instances/${id}/mods/configs`).then((r) => r.files),
    enabled: !!id,
  })
}

/** GET /instances/{id}/mods/configs/{fileName} */
export function useModConfig(id: string, fileName: string | undefined) {
  return useQuery({
    queryKey: ['instances', id, 'mods', 'config', fileName],
    queryFn: () => api.get<ConfigFile>(`/instances/${id}/mods/configs/${encodeURIComponent(fileName!)}`),
    enabled: !!id && !!fileName,
  })
}

/** PUT /instances/{id}/mods/configs/{fileName} — either `raw` or `values`. */
export function useSaveModConfig(id: string, fileName: string | undefined) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: ConfigFileUpdate) =>
      api.put<ConfigFile>(`/instances/${id}/mods/configs/${encodeURIComponent(fileName!)}`, body),
    onSuccess: (file) => {
      qc.setQueryData(['instances', id, 'mods', 'config', fileName], file)
      // Covers the file list (mtime/size changed) and the overview, whose
      // `pending_restart` flips true for a running instance (docs/ARCHITECTURE.md §12).
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess('Config saved')
    },
    onError: (err) => notifyError(err, 'Could not save config'),
  })
}
