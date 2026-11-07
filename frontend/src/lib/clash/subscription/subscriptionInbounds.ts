import type { Subscription, SubscriptionProxy } from '@/schemas/clash/subscription';

export function subscriptionInboundOptions(proxies: SubscriptionProxy[]) {
  const inbounds = new Map<string, { name: string; hostOverride: boolean }>();
  for (const proxy of proxies) {
    if (proxy.inboundKey)
      inbounds.set(proxy.inboundKey, {
        name: proxy.inboundName,
        hostOverride: proxy.hostOverride || (inbounds.get(proxy.inboundKey)?.hostOverride ?? false),
      });
  }
  const names = new Map<string, number>();
  for (const { name } of inbounds.values()) names.set(name, (names.get(name) ?? 0) + 1);
  return [...inbounds].map(([inboundKey, { name, hostOverride }]) => ({
    hostOverride,
    inboundKey,
    name: !name || names.get(name)! > 1 ? `${name || '未命名入站'} (${inboundKey})` : name,
  }));
}

export function expandLegacyNodeBindings(
  binding: Subscription,
  proxies: SubscriptionProxy[],
): Subscription {
  return {
    ...binding,
    groups: binding.groups.map((group) => ({
      ...group,
      nodes: [
        ...new Set(
          group.nodes.flatMap((key) => {
            if (key.startsWith('inbound:')) return [key];
            const inbounds = proxies
              .filter((proxy) => proxy.nodeKey === key && proxy.inboundKey)
              .map((proxy) => proxy.inboundKey);
            return inbounds.length ? inbounds : [key];
          }),
        ),
      ],
    })),
  };
}
