import { parseDocument, stringify } from 'yaml';
import { z } from 'zod';

import type { Subscription } from '@/schemas/clash/subscription';

export type Section = 'rules' | 'rule-providers' | 'proxy-groups';
export const disabledField = {
  rules: 'disabledRules',
  'rule-providers': 'disabledProviders',
  'proxy-groups': 'disabledGroups',
} as const;
export function documentObject(content: string): Record<string, unknown> {
  const doc = parseDocument(content);
  if (doc.errors.length) throw new Error(doc.errors[0].message);
  return z.record(z.string(), z.unknown()).parse(doc.toJS({ maxAliasCount: 100 }));
}
export function ruleIdentity(rule: string): string {
  const first = rule.indexOf(',');
  if (first <= 0) throw new Error('规则缺少类型或目标');
  const kind = rule.slice(0, first).trim().toUpperCase();
  let remainder = rule.slice(first + 1).trim();
  if (kind === 'MATCH') {
    if (!remainder) throw new Error('规则缺少目标');
    return kind;
  }
  while (true) {
    const last = remainder.lastIndexOf(',');
    if (last < 0) throw new Error('规则缺少匹配内容或目标');
    const tail = remainder.slice(last + 1).trim();
    if (['no-resolve', 'src'].includes(tail.toLowerCase())) {
      remainder = remainder.slice(0, last);
      continue;
    }
    const predicate = remainder.slice(0, last).trim();
    if (!predicate || !tail) throw new Error('规则缺少匹配内容或目标');
    return kind + ',' + predicate;
  }
}

export function entries(config: Record<string, unknown>, section: Section): [string, unknown][] {
  const value = config[section];
  if (value === undefined) return [];
  if (section === 'rule-providers')
    return Object.entries(z.record(z.string(), z.unknown()).parse(value));
  if (section === 'rules')
    return z
      .array(z.string())
      .parse(value)
      .map((rule) => [ruleIdentity(rule), rule]);
  return z
    .array(z.object({ name: z.string() }).passthrough())
    .parse(value)
    .map((group) => [group.name, group]);
}
export function setEntries(
  config: Record<string, unknown>,
  section: Section,
  rows: [string, unknown][],
) {
  return {
    ...config,
    [section]:
      section === 'rule-providers' ? Object.fromEntries(rows) : rows.map(([, value]) => value),
  };
}
export function composeSubscription(base: string, binding: Subscription) {
  let config = documentObject(base);
  const overlay = documentObject(binding.content);
  for (const [key, value] of Object.entries(overlay)) {
    if (['rules', 'rule-providers', 'proxy-groups'].includes(key)) continue;
    if (value === null) delete config[key];
    else config[key] = value;
  }
  for (const section of ['rules', 'rule-providers', 'proxy-groups'] as const) {
    const additions = entries(overlay, section);
    if (new Set(additions.map(([id]) => id)).size !== additions.length)
      throw new Error('覆盖项重复：' + section);
    const replacements = new Map(additions);
    const used = new Set<string>();
    const result: [string, unknown][] = [];
    for (const [id, value] of entries(config, section)) {
      if (replacements.has(id)) {
        if (!used.has(id)) {
          result.push([id, replacements.get(id)]);
          used.add(id);
        }
      } else if (!binding[disabledField[section]].includes(id)) result.push([id, value]);
    }
    const added = additions.filter(([id]) => !used.has(id));
    const match = section === 'rules' ? result.findIndex(([id]) => id === 'MATCH') : -1;
    result.splice(match < 0 ? result.length : match, 0, ...added);
    config = setEntries(config, section, result);
  }
  return stringify(config);
}

export function subscriptionFromYAML(
  base: string,
  content: string,
  binding: Subscription,
): Subscription {
  const original = documentObject(base),
    edited = documentObject(content);
  let overlay: Record<string, unknown> = {};
  const next = { ...binding };
  for (const section of ['rules', 'rule-providers', 'proxy-groups'] as const) {
    const inherited = new Map(entries(original, section));
    const rows = entries(edited, section);
    next[disabledField[section]] = [...inherited.keys()].filter(
      (id) => !rows.some(([key]) => key === id),
    );
    overlay = setEntries(
      overlay,
      section,
      rows.filter(
        ([id, value]) => !inherited.has(id) || stringify(inherited.get(id)) !== stringify(value),
      ),
    );
  }
  for (const key of new Set([...Object.keys(original), ...Object.keys(edited)])) {
    if (['rules', 'rule-providers', 'proxy-groups'].includes(key)) continue;
    if (stringify(original[key]) !== stringify(edited[key])) overlay[key] = edited[key] ?? null;
  }
  next.content = stringify(overlay);
  return next;
}
