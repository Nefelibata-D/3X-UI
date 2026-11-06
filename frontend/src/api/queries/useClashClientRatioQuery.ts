import { useQuery } from '@tanstack/react-query';

import { keys } from '@/api/queryKeys';
import { fetchClashClientRatio } from '@/lib/clash/api';

export function useClashClientRatioQuery(clientId: number | undefined) {
  return useQuery({
    queryKey: keys.clash.client(clientId ?? 0),
    refetchInterval: 10_000,
    queryFn: () => fetchClashClientRatio(clientId as number),
    enabled: typeof clientId === 'number' && clientId > 0,
  });
}
