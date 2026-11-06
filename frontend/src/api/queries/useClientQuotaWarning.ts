import { useClashClientRatioQuery } from './useClashClientRatioQuery';
import { clientQuotaSummary } from '@/lib/clash/quota';

export function useClientQuotaWarning(clientId: number | undefined, totalBytes: number) {
  const query = useClashClientRatioQuery(clientId);
  const summary = clientQuotaSummary(
    (query.data?.nodes ?? []).map((node) => node.budgetBytes),
    totalBytes,
  );
  return {
    conflict: summary.unlimited || summary.exceeded,
    error: summary.unlimited
      ? '存在无限节点额度，不能设置有限的客户端总流量'
      : '节点额度合计超过客户端总流量上限',
  };
}
