import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { fetchClashOverview } from '@/lib/clash/allocation/api';

export function useClashOverviewQuery() {
  return useQuery({
    queryKey: keys.clash.overview(),
    queryFn: fetchClashOverview,
    refetchInterval: 10_000,
  });
}
