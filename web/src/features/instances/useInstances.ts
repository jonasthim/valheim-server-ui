import { useQuery } from '@tanstack/react-query'
import { api } from '../../api/client'
import type { Instance } from '../../api/types'

// Shared query for the instance list; the dashboard and the sidebar's
// dynamic "Instances" nav group both reuse this (U-A4).
export function useInstances() {
  return useQuery({
    queryKey: ['instances', 'list'],
    queryFn: () => api.get<{ instances: Instance[] }>('/instances').then((r) => r.instances),
    refetchInterval: 15_000,
  })
}
