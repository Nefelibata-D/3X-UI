import { isMap, isSeq, isScalar, type Document, type Scalar } from 'yaml';

export type SymbolKind = 'proxy' | 'provider' | 'ruleset' | 'subrule' | 'groupType' | 'proxyType';
export const groupTypes = ['select', 'url-test', 'fallback', 'load-balance'];
export const builtins = ['DIRECT', 'REJECT', 'REJECT-DROP', 'PASS', 'GLOBAL', 'COMPATIBLE'];
export interface Slot {
  node: Scalar;
  kind: SymbolKind;
  start: number;
  end: number;
  declaration: boolean;
  name: string;
}

export function templateSymbols(doc: Document) {
  const slots: Slot[] = [];
  function add(node: unknown, kind: SymbolKind, declaration = false, start = 0, end?: number) {
    if (!isScalar(node) || (typeof node.value !== 'string' && node.value !== null)) return;
    const s = String(node.value ?? '');
    slots.push({ node, kind, declaration, start, end: end ?? s.length, name: s.slice(start, end) });
  }
  function sequence(node: unknown, fn: (value: unknown) => void) {
    if (isSeq(node)) node.items.forEach(fn);
  }
  function map(node: unknown, fn: (key: unknown, value: unknown) => void) {
    if (isMap(node)) node.items.forEach((p) => fn(p.key, p.value));
  }
  function policy(node: unknown) {
    if (isMap(node)) add(node.get('dialer-proxy', true), 'proxy');
  }
  function rule(node: unknown, target: boolean) {
    if (!isScalar(node) || typeof node.value !== 'string') return;
    const s = node.value;
    const parts = s.split(',');
    const type = parts[0].trim().toUpperCase();
    function field(index: number, kind: SymbolKind) {
      if (index >= parts.length || index < 1) return;
      const raw = parts[index];
      const start =
        parts.slice(0, index).join(',').length + 1 + raw.length - raw.trimStart().length;
      add(node, kind, false, start, start + raw.trim().length);
    }
    if (type === 'RULE-SET') field(1, 'ruleset');
    const compound = ['AND', 'OR', 'NOT', 'SUB-RULE'].includes(type);
    function scanChild(start: number, end: number) {
      const comma = s.indexOf(',', start);
      if (comma < start || comma >= end) return;
      const childType = s.slice(start, comma).trim().toUpperCase();
      if (childType === 'RULE-SET') {
        const next = s.indexOf(',', comma + 1);
        const raw = s.slice(comma + 1, next >= 0 && next < end ? next : end);
        const first = comma + 1 + raw.length - raw.trimStart().length;
        add(node, 'ruleset', false, first, first + raw.trim().length);
      } else if (['AND', 'OR', 'NOT'].includes(childType)) scanList(comma + 1, end);
    }
    function scanList(start: number, end: number) {
      let depth = 0,
        opener = 0;
      for (let i = start; i < end; i++) {
        if (s[i] === '(') {
          depth++;
          if (depth === 2) opener = i + 1;
        }
        if (s[i] === ')') {
          if (depth === 2) scanChild(opener, i);
          depth--;
        }
      }
    }
    if (compound) {
      const start = s.indexOf(',') + 1;
      const end = target ? s.lastIndexOf(',') : s.length;
      if (type === 'SUB-RULE') {
        const payload = s.slice(start, end).trim();
        if (payload.startsWith('(') && payload.endsWith(')')) {
          const first = s.indexOf('(', start);
          scanChild(first + 1, first + payload.length - 1);
        }
      } else scanList(start, end);
    }
    if (target)
      field(
        type === 'MATCH'
          ? 1
          : compound || ['DOMAIN-REGEX', 'PROCESS-NAME-REGEX', 'PROCESS-PATH-REGEX'].includes(type)
            ? parts.length - 1
            : 2,
        type === 'SUB-RULE' ? 'subrule' : 'proxy',
      );
  }
  policy(doc.get('ntp'));
  sequence(doc.get('proxies'), (p) => {
    if (isMap(p)) {
      add(p.get('name', true), 'proxy', true);
      add(p.get('type', true), 'proxyType');
      policy(p);
    }
  });
  sequence(doc.get('proxy-groups'), (g) => {
    if (isMap(g)) {
      add(g.get('name', true), 'proxy', true);
      add(g.get('type', true), 'groupType');
      sequence(g.get('proxies'), (p) => add(p, 'proxy'));
      sequence(g.get('use'), (p) => add(p, 'provider'));
      map(g.getIn(['weighted-sticky', 'weights']), (key) => add(key, 'proxy'));
      map(g.getIn(['weighted-sticky', 'resilience', 'control', 'node-ids']), (key) =>
        add(key, 'proxy'),
      );
    }
  });
  map(doc.get('proxy-providers'), (key, p) => {
    add(key, 'provider', true);
    policy(p);
    if (isMap(p)) {
      policy(p.get('override'));
      add(p.get('proxy', true), 'proxy');
    }
  });
  map(doc.get('rule-providers'), (key, p) => {
    add(key, 'ruleset', true);
    if (isMap(p)) add(p.get('proxy', true), 'proxy');
  });
  sequence(doc.get('rules'), (r) => rule(r, true));
  map(doc.get('sub-rules'), (k, v) => {
    add(k, 'subrule', true);
    sequence(v, (r) => rule(r, true));
  });
  sequence(doc.get('site-blacklist'), (b) => {
    if (isMap(b)) {
      add(b.get('proxy', true), 'proxy');
      add(b.get('group', true), 'proxy');
      sequence(b.get('rules'), (r) => rule(r, false));
    }
  });
  for (const key of ['nameserver-policy', 'proxy-server-nameserver-policy']) {
    map(doc.getIn(['dns', key]), (k) => {
      if (
        !isScalar(k) ||
        typeof k.value !== 'string' ||
        !k.value.toLowerCase().startsWith('rule-set:')
      )
        return;
      let start = 9;
      for (const name of k.value.slice(start).split(',')) {
        add(k, 'ruleset', false, start, start + name.length);
        start += name.length + 1;
      }
    });
  }
  return slots;
}
export function replaceSymbol(slots: Slot[], kind: SymbolKind, oldName: string, newName: string) {
  for (const slot of slots
    .filter((s) => s.kind === kind && s.name === oldName)
    .sort((a, b) => b.start - a.start)) {
    const value = String(slot.node.value);
    slot.node.value = value.slice(0, slot.start) + newName + value.slice(slot.end);
  }
}
