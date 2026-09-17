// Resource/player history hooks (F-1.3): feed the Overview "History" section
// and the compact CPU/Memory sparklines, and the dashboard's host tiles.
import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { MetricSeries } from '../../api/types'

export type MetricRange = '1h' | '24h' | '7d' | '30d'

/** GET /instances/{id}/metrics?range=, polled every 60s alongside the sampler's own tick. */
export function useInstanceMetrics(id: string, range: MetricRange) {
  return useQuery({
    queryKey: ['instances', id, 'metrics', range],
    queryFn: () => api.get<MetricSeries>(`/instances/${id}/metrics?range=${range}`),
    enabled: !!id,
    refetchInterval: 60_000,
  })
}

/** GET /system/metrics?range=, the dashboard's host CPU/Memory sparklines. */
export function useSystemMetrics(range: MetricRange) {
  return useQuery({
    queryKey: ['system', 'metrics', range],
    queryFn: () => api.get<MetricSeries>(`/system/metrics?range=${range}`),
    refetchInterval: 60_000,
  })
}
