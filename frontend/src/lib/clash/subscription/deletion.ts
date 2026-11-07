import { isMap, isSeq, parseDocument, stringify } from 'yaml';
import { templateSymbols } from '@/lib/clash/subscription/template/symbols';
import {
  composeSubscription,
  documentObject,
  entries,
  setEntries,
  subscriptionFromYAML,
} from '@/lib/clash/subscription/model';
import { type Subscription } from '@/schemas/clash/subscription';

export function deleteSubscriptionGroup(base: string, binding: Subscription, name: string) {
  const overlay = documentObject(binding.content);
  const restored = entries(documentObject(base), 'proxy-groups').some(([id]) => id === name);
  if (restored) {
    return {
      restored,
      references: [] as string[],
      binding: {
        ...binding,
        disabledGroups: binding.disabledGroups.filter((id) => id !== name),
        content: stringify(
          setEntries(
            overlay,
            'proxy-groups',
            entries(overlay, 'proxy-groups').filter(([id]) => id !== name),
          ),
        ),
      },
    };
  }

  const doc = parseDocument(composeSubscription(base, binding));
  const groups = doc.get('proxy-groups');
  if (!isSeq(groups)) throw new Error('节点组列表无效');
  const index = groups.items.findIndex((group) => isMap(group) && group.get('name') === name);
  if (index < 0) throw new Error('节点组不存在');
  groups.delete(index);
  const targets = new Set(
    templateSymbols(doc)
      .filter((slot) => slot.kind === 'proxy' && !slot.declaration && slot.name === name)
      .map((slot) => slot.node),
  );
  const references: string[] = [];
  const blacklists = doc.get('site-blacklist');
  if (isSeq(blacklists)) {
    for (let i = blacklists.items.length - 1; i >= 0; i--) {
      const entry = blacklists.items[i];
      if (isMap(entry) && (entry.get('group') === name || entry.get('proxy') === name)) {
        references.push('黑名单：' + stringify(entry.toJSON()).trim());
        blacklists.delete(i);
      }
    }
  }
  const labels: Record<string, string> = {
    rules: '路由',
    'sub-rules': '子路由',
    'proxy-groups': '节点组',
    'rule-providers': '规则集',
    'proxy-providers': '代理集合',
    proxies: '代理',
  };
  function remove(node: unknown, path: string) {
    if (isSeq(node)) {
      for (let i = node.items.length - 1; i >= 0; i--) {
        const item = node.items[i];
        if ([...targets].some((target) => target === item)) {
          references.push(path + '：' + String(item));
          node.delete(i);
        } else
          remove(
            item,
            path + (isMap(item) && item.has('name') ? ' / ' + String(item.get('name')) : ''),
          );
      }
    } else if (isMap(node)) {
      for (let i = node.items.length - 1; i >= 0; i--) {
        const pair = node.items[i];
        const key = String(pair.key);
        const next = path ? path + ' / ' + key : (labels[key] ?? key);
        if ([...targets].some((target) => target === pair.key || target === pair.value)) {
          references.push(next + '：' + String(pair.value));
          node.items.splice(i, 1);
        } else remove(pair.value, next);
      }
    }
  }
  remove(doc.contents, '');
  const edited = documentObject(doc.toString());
  const next = subscriptionFromYAML(base, doc.toString(), binding);
  let changes = documentObject(next.content);
  for (const section of ['rules', 'rule-providers', 'proxy-groups'] as const) {
    const previous = new Map(entries(overlay, section));
    const modified = new Map(entries(changes, section));
    changes = setEntries(
      changes,
      section,
      entries(edited, section).filter(
        ([id, value]) =>
          modified.has(id) ||
          (previous.has(id) && stringify(previous.get(id)) === stringify(value)),
      ),
    );
  }
  for (const [key, value] of Object.entries(overlay)) {
    if (['rules', 'rule-providers', 'proxy-groups'].includes(key)) continue;
    if (stringify(edited[key]) === stringify(value)) changes[key] = value;
  }
  next.content = stringify(changes);
  next.disabledRules = [...new Set([...binding.disabledRules, ...next.disabledRules])];
  next.disabledProviders = [...new Set([...binding.disabledProviders, ...next.disabledProviders])];
  next.disabledGroups = [...new Set([...binding.disabledGroups, ...next.disabledGroups])].filter(
    (id) => id !== name,
  );
  next.groups = binding.groups.filter((group) => group.group !== name);
  return { restored, references, binding: next };
}
