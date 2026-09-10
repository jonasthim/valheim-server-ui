// Queries for the live map: GET /instances/{id}/map (render state, objects,
// players) and the re-render mutation. Player positions also arrive live via
// the agent.status SSE event (see features/agent/useAgent).
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { API_BASE, api } from '../../api/client'
import type { InstanceMap, MapInfo, MapRenderRequest } from '../../api/types'
import { notifyError, notifySuccess } from '../../lib/notify'

export function mapKey(id: string) {
  return ['instances', id, 'map'] as const
}

export function useInstanceMap(id: string) {
  return useQuery({
    queryKey: mapKey(id),
    queryFn: () => api.get<InstanceMap>(`/instances/${id}/map`),
    enabled: !!id,
    refetchInterval: (q) => {
      const st = q.state.data?.info?.state
      return st === 'rendering' || st === 'encoding' ? 3_000 : 5_000
    },
  })
}

export function useRenderMap(id: string) {
  const qc = useQueryClient()
  return useMutation({
    mutationFn: (req: MapRenderRequest) => api.post<MapInfo>(`/instances/${id}/map/render`, req),
    onSuccess: () => {
      notifySuccess('Map render started on the server')
      void qc.invalidateQueries({ queryKey: mapKey(id) })
    },
    onError: (err) => notifyError(err, 'Could not start the render'),
  })
}

/** Image URL; the version suffix busts the browser cache after a re-render. */
/**
 * Map image URL. The fog is composited on the server, so the same endpoint
 * serves the fogged image (everyone) or, with fog=0, the bare render
 * (operators). `version` is InstanceMap.image_version, which changes on
 * every render and fog rebuild.
 */
export function mapImageUrl(id: string, version: string | undefined, fog: boolean): string {
  const v = version || 'cached'
  const q = `v=${encodeURIComponent(v)}${fog ? '' : '&fog=0'}`
  return `${API_BASE}/instances/${encodeURIComponent(id)}/map.png?${q}`
}

/** World coordinates to image fractions (0..1), north up. */
export function worldToFraction(x: number, z: number, radius: number): { u: number; v: number } {
  const r = radius > 0 ? radius : 10500
  return { u: (x + r) / (2 * r), v: (r - z) / (2 * r) }
}
