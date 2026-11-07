import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useClashClientRatioQuery } from './useClashClientRatioQuery';
import * as api from '@/lib/clash/subscription/api';
import type { Subscription } from '@/schemas/clash/subscription';

export function useClashRuleSets(enabled = true) {
  return useQuery({ queryKey: keys.clash.rulesets(), queryFn: api.fetchRuleSets, enabled });
}
export function useClashTemplates() {
  return useQuery({ queryKey: keys.clash.templates(), queryFn: api.fetchTemplates });
}
export function useClashSubscription(id: number | undefined) {
  const binding = useQuery({
    queryKey: keys.clash.subscription(id ?? 0),
    queryFn: () => api.fetchSubscription(id),
    enabled: id !== undefined && id > 0,
  });
  const proxies = useQuery({
    queryKey: keys.clash.proxies(id ?? 0),
    queryFn: () => api.fetchSubscriptionProxies(id),
    enabled: id !== undefined && id > 0,
  });
  const templates = useClashTemplates();
  const rules = useClashRuleSets();
  const ratio = useClashClientRatioQuery(id);
  const queries = [binding, proxies, templates, rules, ratio];
  return {
    isPending: queries.some((query) => query.isPending),
    error: !id ? new Error('客户端没有有效 ID') : queries.find((query) => query.error)?.error,
    data:
      binding.data && proxies.data && templates.data && rules.data && ratio.data
        ? {
            binding: binding.data,
            proxies: proxies.data,
            templates: templates.data,
            rules: rules.data,
            nodes: ratio.data.nodes.filter((node) => node.selectable),
          }
        : undefined,
  };
}
export function useSaveClashRuleSet() {
  const cache = useQueryClient();
  return useMutation({
    mutationFn: api.saveRuleSet,
    onSuccess: () => cache.invalidateQueries({ queryKey: keys.clash.rulesets() }),
  });
}
export function useDeleteClashRuleSet() {
  const cache = useQueryClient();
  return useMutation({
    mutationFn: api.deleteRuleSet,
    onSuccess: () => cache.invalidateQueries({ queryKey: keys.clash.rulesets() }),
  });
}
export function useClashTemplateMutations() {
  const cache = useQueryClient();
  const invalidate = () =>
    Promise.all([
      cache.invalidateQueries({ queryKey: keys.clash.templates() }),
      cache.invalidateQueries({ queryKey: keys.clash.subscriptions() }),
    ]);
  const save = useMutation({ mutationFn: api.saveTemplate, onSuccess: invalidate });
  const remove = useMutation({ mutationFn: api.deleteTemplate, onSuccess: invalidate });
  return { save, remove };
}
export function useSaveClashSubscription(id: number | undefined) {
  const cache = useQueryClient();
  return useMutation({
    mutationFn: (row: Subscription) => api.saveSubscription(id, row),
    onSuccess: () => cache.invalidateQueries({ queryKey: keys.clash.subscription(id ?? 0) }),
  });
}
