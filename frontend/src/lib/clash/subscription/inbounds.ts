import type { Subscription, SubscriptionProxy } from '@/schemas/clash/subscription';

export interface SubscriptionInboundOption {
  inboundKey: string;
  name: string;
  hostOverride: boolean;
  proxyNames?: string[];
}

export function subscriptionInboundOptions(
  proxies: SubscriptionProxy[],
): SubscriptionInboundOption[] {
  const inbounds = new Map<string, SubscriptionInboundOption>();
  for (const proxy of proxies) {
    if (!proxy.inboundKey) continue;
    const option = inbounds.get(proxy.inboundKey) ?? {
      inboundKey: proxy.inboundKey,
      name: '',
      hostOverride: false,
      proxyNames: [],
    };
    option.proxyNames!.push(proxy.name);
    option.hostOverride ||= proxy.hostOverride;
    inbounds.set(proxy.inboundKey, option);
  }
  return [...inbounds.values()].map((option) => ({
    ...option,
    name: option.proxyNames!.join('、'),
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
