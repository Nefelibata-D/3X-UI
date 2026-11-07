import { isMap, isSeq, parseDocument } from 'yaml';
import { proxyTypes } from '@/lib/clash/subscription/template/schema';
import { builtins } from '@/lib/clash/subscription/template/symbols';

export function weightedGroupIssue(
  members: string[],
  groupNames: string[],
  managedProxyNames?: string[],
) {
  const nested = members.filter((name) => groupNames.includes(name));
  if (nested.length) return '加权负载均衡不能包含其他代理组：' + nested.join('、');
  const system = members.filter((name) => builtins.includes(name));
  if (system.length) return '加权负载均衡不能查询系统出站的服务器额度：' + system.join('、');
  if (managedProxyNames) {
    const unknown = members.filter((name) => !managedProxyNames.includes(name));
    if (unknown.length) return '代理无法关联授权面板：' + unknown.join('、');
    if (new Set(members).size < 2) return '请先为代理组分配至少两个不同的受管代理';
  }
  return '';
}

export function loadBalanceModel(content: string) {
  const doc = parseDocument(content);
  const proxies = doc.get('proxies');
  const groups = doc.get('proxy-groups');
  const actual = isSeq(proxies)
    ? proxies.items
        .filter(isMap)
        .filter(
          (p) =>
            proxyTypes.includes(String(p.get('type'))) &&
            !['direct', 'reject', 'dns'].includes(String(p.get('type'))),
        )
        .map((p) => String(p.get('name')))
    : [];
  const rows = isSeq(groups)
    ? groups.items.flatMap((g, index) => {
        if (!isMap(g)) return [];
        const members = g.get('proxies');
        const names = isSeq(members) ? members.items.map(String) : [];
        const candidates = new Set(names.filter((n) => actual.includes(n)));
        const weighted = g.get('weighted-sticky');
        const effective = [...candidates].filter(
          (n) =>
            !isMap(weighted) ||
            Number(weighted.getIn(['weights', n]) ?? weighted.get('default-weight') ?? 1) > 0,
        );
        return [
          {
            index,
            name: String(g.get('name')),
            type: String(g.get('type')),
            strategy: String(g.get('strategy') ?? ''),
            members: names,
            effective,
          },
        ];
      })
    : [];
  return { doc, actual: [...new Set(actual)], groups: rows };
}

export const blacklistRuleKinds = [
  'DOMAIN',
  'DOMAIN-SUFFIX',
  'DOMAIN-KEYWORD',
  'DOMAIN-REGEX',
  'DOMAIN-WILDCARD',
  'PROCESS-NAME',
  'PROCESS-PATH',
  'PROCESS-NAME-REGEX',
  'PROCESS-PATH-REGEX',
  'PROCESS-NAME-WILDCARD',
  'PROCESS-PATH-WILDCARD',
];
export function isBlacklistRule(line: string) {
  const comma = line.indexOf(',');
  const kind = line.slice(0, comma);
  const value = line.slice(comma + 1).trim();
  return (
    comma > 0 &&
    blacklistRuleKinds.includes(kind) &&
    !!value &&
    !/[\r\n]/.test(value) &&
    (kind.endsWith('-REGEX') || !value.includes(','))
  );
}

export function loadBalanceErrors(
  content: string,
  stage: 'template' | 'subscription' = 'template',
  onError?: (message: string, node: unknown) => void,
) {
  const { doc, groups, actual } = loadBalanceModel(content);
  const errors: string[] = [];
  const add = (message: string, node: unknown) => {
    errors.push(message);
    onError?.(message, node);
  };
  for (const group of groups) {
    if (group.type !== 'load-balance' || group.strategy !== 'weighted-sticky') continue;
    const issue = weightedGroupIssue(
      group.members,
      groups.map((g) => g.name),
    );
    if (issue) add(`${group.name}：${issue}`, doc.getIn(['proxy-groups', group.index], true));
  }
  for (const g of groups)
    if (stage === 'subscription' && g.type === 'load-balance' && g.effective.length < 2)
      add(
        `${g.name}：负载均衡至少需要两个不同的有效代理节点，DIRECT、策略组及零权重节点不计入。`,
        doc.getIn(['proxy-groups', g.index], true),
      );
  const entries = doc.get('site-blacklist');
  if (isSeq(entries))
    for (const entry of entries.items) {
      if (!isMap(entry)) continue;
      const scope = entry.get('group');
      const group = groups.find((g) => g.name === scope);
      if (
        scope &&
        (!group || group.type !== 'load-balance' || group.strategy !== 'weighted-sticky')
      )
        add(`黑名单所属组 ${scope} 必须开启 weighted-sticky 负载均衡。`, entry.get('group', true));
      const proxy = String(entry.get('proxy'));
      if (
        stage === 'subscription' &&
        (!actual.includes(proxy) || (group && !group.effective.includes(proxy)))
      )
        add(`黑名单节点 ${proxy} 必须是所属组中的有效代理。`, entry.get('proxy', true));
      const rules = entry.get('rules');
      if (!isSeq(rules)) continue;
      for (const rule of rules.items) {
        const line = String(rule);
        const comma = line.indexOf(',');
        const kind = line.slice(0, comma);
        const payload = line.slice(comma + 1);
        if (
          !isBlacklistRule(line) &&
          !(kind === 'RULE-SET' && payload.trim() && !payload.includes(','))
        )
          add(`黑名单只支持 DOMAIN / PROCESS 系列规则和 RULE-SET：${rule}`, rule);
        if (kind === 'RULE-SET') {
          const provider = doc.getIn(['rule-providers', payload]);
          if (
            !isMap(provider) ||
            provider.get('behavior') !== 'classical' ||
            provider.get('format') === 'mrs'
          )
            add(`黑名单规则集 ${payload} 必须是 classical 类型的 YAML 或文本规则集。`, rule);
          const items = isMap(provider) ? provider.get('payload') : undefined;
          if (isSeq(items) && items.items.some((r) => !isBlacklistRule(String(r))))
            add(`黑名单规则集 ${payload} 只能包含 DOMAIN / PROCESS 系列规则。`, rule);
        }
      }
    }
  for (const group of groups.filter((g) => g.type === 'load-balance')) {
    const excluded = new Set<string>();
    if (isSeq(entries))
      for (const entry of entries.items) {
        if (isMap(entry) && (!entry.get('group') || entry.get('group') === group.name))
          excluded.add(String(entry.get('proxy')));
      }
    if (
      stage === 'subscription' &&
      excluded.size &&
      !group.effective.some((name) => !excluded.has(name))
    )
      add(
        `${group.name}：必须保留至少一个未配置黑名单的有效代理节点。`,
        doc.getIn(['proxy-groups', group.index], true),
      );
  }
  return errors;
}
