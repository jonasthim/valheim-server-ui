// Query/mutation hooks for the instance-scoped mods API (BepInEx, installed
// mods, Thunderstore install, uploads). Kept separate from components so the
// barrel export doesn't trip oxlint's react/only-export-components rule.
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Job, Mod, ModsOverview, StopIfRunning } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

/** GET /instances/{id}/mods -> BepInEx status + installed mod list. */
export function useModsOverview(id: string) {
  return useQuery({
    queryKey: ['instances', id, 'mods'],
    queryFn: () => api.get<ModsOverview>(`/instances/${id}/mods`),
    enabled: !!id,
  })
}

/** POST /instances/{id}/mods/bepinex — install or upgrade the mod loader. */
export function useInstallBepinex(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: StopIfRunning) => api.post<{ job: Job }>(`/instances/${id}/mods/bepinex`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess('BepInEx install queued')
    },
    onError: (err) => notifyError(err, 'Could not install BepInEx'),
  })
}

/** PATCH /instances/{id}/mods/bepinex {enabled} */
export function useSetBepinexEnabled(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (enabled: boolean) => api.patch<ModsOverview>(`/instances/${id}/mods/bepinex`, { enabled }),
    onSuccess: (overview) => {
      qc.setQueryData(['instances', id, 'mods'], overview)
      notifySuccess(overview.bepinex.enabled ? 'BepInEx enabled' : 'BepInEx disabled')
    },
    onError: (err) => notifyError(err, 'Could not update BepInEx'),
  })
}

/** PATCH /instances/{id}/mods/{modId} {enabled} */
export function useSetModEnabled(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ modId, enabled }: { modId: number; enabled: boolean }) =>
      api.patch<{ mod: Mod }>(`/instances/${id}/mods/${modId}`, { enabled }),
    onSuccess: (res) => {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess(res.mod.enabled ? `${res.mod.name} enabled` : `${res.mod.name} disabled`)
    },
    onError: (err) => notifyError(err, 'Could not update mod'),
  })
}

/** POST /instances/{id}/mods/{modId}/update — update one Thunderstore mod. */
export function useUpdateMod(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ modId, version }: { modId: number; version?: string }) =>
      api.post<{ job: Job }>(`/instances/${id}/mods/${modId}/update`, version ? { version } : undefined),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess('Update queued')
    },
    onError: (err) => notifyError(err, 'Could not queue mod update'),
  })
}

/** DELETE /instances/{id}/mods/{modId} */
export function useUninstallMod(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: ({ modId, removeConfigs = [] }: { modId: number; removeConfigs?: string[] }) => {
      const qs = removeConfigs.map((c) => `remove_config=${encodeURIComponent(c)}`).join('&')
      return api.del<{ job: Job }>(`/instances/${id}/mods/${modId}${qs ? `?${qs}` : ''}`)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess('Uninstall queued')
    },
    onError: (err) => notifyError(err, 'Could not uninstall mod'),
  })
}

/** POST /instances/{id}/mods/upload (multipart, field `file`). */
export function useUploadMod(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (file: File) => {
      const form = new FormData()
      form.set('file', file)
      return api.upload<{ job: Job }>(`/instances/${id}/mods/upload`, form)
    },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess('Mod upload queued')
    },
    onError: (err) => notifyError(err, 'Could not upload mod'),
  })
}

/** POST /instances/{id}/mods {registry?, owner, name, version?} — install a package. */
export function useInstallPackage(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (body: { registry?: string; owner: string; name: string; version?: string }) =>
      api.post<{ job: Job }>(`/instances/${id}/mods`, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: ['instances', id, 'mods'] })
      notifySuccess('Install queued')
    },
    onError: (err) => notifyError(err, 'Could not queue install'),
  })
}
