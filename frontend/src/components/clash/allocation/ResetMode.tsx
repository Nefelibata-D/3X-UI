import { useMutation, useQueryClient } from '@tanstack/react-query';
import { Select, message } from 'antd';

import { keys } from '@/api/queryKeys';
import { useClashClientRatioQuery } from '@/api/queries/clash/allocation/useClientRatio';
import { saveClashClientRatio } from '@/lib/clash/allocation/api';

export default function ClientNodeResetMode({ clientId }: { clientId?: number }) {
  const queryClient = useQueryClient();
  const query = useClashClientRatioQuery(clientId);
  const mutation = useMutation({
    mutationFn: (resetMode: 'node' | 'manual') => {
      if (!clientId || !query.data) throw new Error('客户端节点策略尚未加载');
      const weights = Object.fromEntries(query.data.nodes.map((node) => [
        node.nodeKey,
        query.data.mode === 'custom' ? node.overrideWeight : node.defaultWeight,
      ]).filter(([, weight]) => Number(weight) > 0));
      const limits = Object.fromEntries(query.data.nodes.map((node) => [node.nodeKey, node.budgetBytes]));
      return saveClashClientRatio(clientId, query.data.mode, weights, limits, resetMode);
    },
    onSuccess: (view) => {
      if (clientId) queryClient.setQueryData(keys.clash.client(clientId), view);
      queryClient.invalidateQueries({ queryKey: keys.clash.overview() });
      message.success('节点流量重置方式已保存。');
    },
    onError: (error: Error) => message.error(error.message),
  });

  return (
    <Select
      value={query.data?.resetMode ?? 'node'}
      loading={query.isLoading || mutation.isPending}
      disabled={!query.data || mutation.isPending}
      onChange={(value: 'node' | 'manual') => mutation.mutate(value)}
      style={{ width: '100%' }}
      aria-label="节点流量重置方式"
      options={[
        { value: 'node', label: '节点流量按节点周期重置' },
        { value: 'manual', label: '节点流量仅手动重置' },
      ]}
    />
  );
}
