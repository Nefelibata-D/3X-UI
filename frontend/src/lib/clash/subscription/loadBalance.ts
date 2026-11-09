import { parseBlacklistRule } from '@/lib/clash/subscription/blacklist';
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
  if (system.length)
    return (
      '当前受管加权组只支持关联面板节点的代理；系统出站不参与服务器额度管理：' + system.join('、')
    );
  if (managedProxyNames) {
    const unknown = members.filter((name) => !managedProxyNames.includes(name));
    if (unknown.length) return '代理无法关联授权面板：' + unknown.join('、');
    if (new Set(members).size < 1) return '请先为代理组分配至少一个受管代理';
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
        const effective = [...candidates];
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
    const options = doc.getIn(['proxy-groups', group.index, 'weighted-sticky']);
    if (isMap(options)) {
      if (
        options.has('weights') ||
        options.has('default-weight') ||
        options.hasIn(['resilience', 'control', 'node-ids'])
      )
        add(`${group.name}：旧加权格式已移除，请迁移到 nodes。`, options);
      const nodes = options.get('nodes');
      if (isSeq(nodes)) {
        const owners = new Set<string>(),
          ids = new Set<string>();
        let total = 0;
        for (const node of nodes.items) {
          if (!isMap(node)) continue;
          const id = String(node.get('id') ?? ''),
            weight = Number(node.get('weight'));
          if (!id || /[/\s]/.test(id) || ids.has(id)) add('服务器 id 必须有效且唯一。', node);
          ids.add(id);
          if (!Number.isSafeInteger(weight) || weight <= 0) add('服务器权重必须为正整数。', node);
          total += weight;
          const proxies = node.get('proxies');
          if (isSeq(proxies))
            for (const proxy of proxies.items) {
              const name = String(proxy);
              if (owners.has(name) || !group.members.includes(name))
                add(`代理 ${name} 归属重复或不在本组。`, proxy);
              owners.add(name);
            }
        }
        if (!Number.isSafeInteger(total)) add('服务器权重总和超出安全整数范围。', nodes);
        for (const member of group.members)
          if (!owners.has(member)) add(`nodes 缺少代理 ${member}。`, nodes);
      }
    }
    const issue = weightedGroupIssue(
      group.members,
      groups.map((g) => g.name),
    );
    if (issue) add(`${group.name}：${issue}`, doc.getIn(['proxy-groups', group.index], true));
  }
  for (const g of groups)
    if (
      stage === 'subscription' &&
      g.type === 'load-balance' &&
      g.effective.length < (g.strategy === 'weighted-sticky' ? 1 : 2)
    )
      add(
        `${g.name}：负载均衡至少需要 ${g.strategy === 'weighted-sticky' ? 1 : 2} 个有效代理，DIRECT 和策略组不计入。`,
        doc.getIn(['proxy-groups', g.index], true),
      );
  const entries = doc.get('site-blacklist');
  const scopes = new Set<string>();
  if (isSeq(entries))
    for (const entry of entries.items) {
      if (!isMap(entry)) continue;
      const scope = entry.get('group');
      if (typeof scope !== 'string' || !scope.trim() || scopes.has(scope))
        add('黑名单 group 必须非空且不能重复。', entry);
      scopes.add(String(scope));
      if (entry.has('proxy'))
        add('旧黑名单 proxy 格式已移除，请使用 类型,内容,[节点列表]。', entry);
      const group = groups.find((g) => g.name === scope);
      if (
        scope &&
        (!group || group.type !== 'load-balance' || group.strategy !== 'weighted-sticky')
      )
        add(`黑名单所属组 ${scope} 必须开启 weighted-sticky 负载均衡。`, entry.get('group', true));
      const rules = entry.get('rules');
      if (!isSeq(rules) || !rules.items.length) {
        add('黑名单 rules 必须是非空列表。', entry);
        continue;
      }
      for (const rule of rules.items) {
        let parsed;
        try {
          parsed = parseBlacklistRule(String(rule));
        } catch (error) {
          add(String(error), rule);
          continue;
        }
        if (stage === 'subscription' && !parsed.proxies.length)
          add('发布前请配置黑名单节点。', rule);
        for (const proxy of parsed.proxies)
          if (
            stage === 'subscription' &&
            (!actual.includes(proxy) || !group?.effective.includes(proxy))
          )
            add(`黑名单节点 ${proxy} 必须是所属组中的有效代理。`, rule);
        const line = parsed.rule;
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
  return errors;
}
