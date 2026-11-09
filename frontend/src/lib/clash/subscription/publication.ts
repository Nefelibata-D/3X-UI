import type { PublicationContext } from '@/schemas/clash/subscription';

function object(value: unknown): Record<string, unknown> {
  if (value === undefined) return {};
  if (!value || typeof value !== 'object' || Array.isArray(value))
    throw Error('负载均衡参数必须为对象');
  return value as Record<string, unknown>;
}

export function restorePublicationFields(
  edited: Record<string, unknown>,
  source: Record<string, unknown>,
  preview: Record<string, unknown>,
) {
  const restore = (
    target: Record<string, unknown>,
    original: Record<string, unknown>,
    key: string,
  ) => {
    if (Object.hasOwn(original, key)) target[key] = original[key];
    else delete target[key];
  };
  for (const key of ['url', 'expected-status', 'interval', 'lazy']) {
    if (!Object.hasOwn(source, key) && edited[key] === preview[key]) delete edited[key];
  }
  if (edited.strategy !== 'weighted-sticky') return;
  const options = object(edited['weighted-sticky']);
  const original = object(source['weighted-sticky']);
  const generated = object(preview['weighted-sticky']);
  const same = (a: unknown, b: unknown): boolean => {
    if (a === b) return true;
    if (!a || !b || typeof a !== 'object' || typeof b !== 'object') return false;
    const left = Object.entries(a),
      right = Object.entries(b);
    return (
      left.length === right.length &&
      left.every(
        ([key, value]) => Object.hasOwn(b, key) && same(value, (b as Record<string, unknown>)[key]),
      )
    );
  };
  if (Object.hasOwn(generated, 'nodes') && !same(options.nodes, generated.nodes))
    throw Error('服务器身份、代理归属和权重由发布器生成，请在节点和负载分配中修改');
  restore(options, original, 'nodes');
  for (const [key, value] of [
    ['idle-timeout', 300],
    ['max-bindings', 10000],
  ] as const) {
    if (!Object.hasOwn(original, key) && options[key] === value) delete options[key];
  }
  const resilience = object(options.resilience),
    originalResilience = object(original.resilience);
  const control = object(resilience.control),
    originalControl = object(originalResilience.control);
  const generatedControl = object(object(generated.resilience).control);
  for (const key of ['base-url', 'uuid']) {
    if (Object.hasOwn(generatedControl, key) && !same(control[key], generatedControl[key]))
      throw Error('控制地址和节点身份由官方订阅生成，不能在 YAML 中覆盖');
  }
  for (const key of ['base-url', 'uuid']) restore(control, originalControl, key);
  for (const [key, value] of [
    ['timeout', 3],
    ['min-interval', 30],
    ['grace-interval', 300],
  ] as const) {
    if (!Object.hasOwn(originalControl, key) && control[key] === value) delete control[key];
  }
  restore(resilience, originalResilience, 'enabled');
  if (Object.keys(control).length) resilience.control = control;
  else delete resilience.control;
  if (Object.keys(resilience).length) options.resilience = resilience;
  else delete options.resilience;
  if (Object.keys(options).length) edited['weighted-sticky'] = options;
  else delete edited['weighted-sticky'];
}

export function injectPublication<T extends Record<string, unknown>>(
  input: T,
  context: PublicationContext,
  nodeKeys: Record<string, string>,
): T {
  const config = structuredClone(input);
  const groups = config['proxy-groups'];
  if (!Array.isArray(groups)) return config;
  for (const value of groups) {
    const group = object(value);
    if (group.type !== 'load-balance') continue;
    const address = new URL(context.baseURL);
    if (
      !['http:', 'https:'].includes(address.protocol) ||
      address.username ||
      address.password ||
      address.search ||
      address.hash
    )
      throw Error('control 地址必须是无凭据、查询参数和片段的 HTTP(S) 地址');
    if (!Object.hasOwn(group, 'url')) {
      group.url = context.baseURL.replace(/\/+$/, '') + '/health';
      group['expected-status'] = 204;
    }
    if (!Object.hasOwn(group, 'interval'))
      group.interval = group.strategy === 'weighted-sticky' ? 1800 : 300;
    if (!Object.hasOwn(group, 'lazy')) group.lazy = false;
    if (group.strategy !== 'weighted-sticky') continue;
    if (context.error) throw Error(context.error);
    if (
      (Array.isArray(group.use) && group.use.length) ||
      group['include-all'] ||
      group['include-all-proxies'] ||
      group['include-all-providers']
    )
      throw Error(`加权组 ${group.name} 必须显式选择受管代理`);
    const nodes: Array<{ name: string; id: string; weight: number; proxies: string[] }> = [];
    const servers = new Map<string, (typeof nodes)[number]>();
    const seen = new Set<string>();
    for (const name of Array.isArray(group.proxies) ? group.proxies : []) {
      if (typeof name !== 'string' || seen.has(name)) throw Error('组成员必须是唯一的代理名称');
      seen.add(name);
      const node = context.nodes[nodeKeys[name]];
      if (!node) throw Error(`加权组 ${group.name} 的成员 ${name} 没有可用的授权面板身份`);
      if (!Number.isSafeInteger(node.weight) || node.weight <= 0)
        throw Error('节点权重必须为安全范围内的正整数');
      let server = servers.get(node.id);
      if (!server) {
        server = { ...node, proxies: [] };
        servers.set(node.id, server);
        nodes.push(server);
      }
      server.proxies.push(name);
    }
    if (nodes.length > 32) throw Error('加权组最多支持 32 个服务器');
    const options = object(group['weighted-sticky']);
    const resilience = object(options.resilience);
    if (
      Object.hasOwn(options, 'weights') ||
      Object.hasOwn(options, 'default-weight') ||
      Object.hasOwn(object(resilience.control), 'node-ids')
    )
      throw Error(
        '旧加权格式已移除，请删除 weights、default-weight 和 control.node-ids，改用 nodes',
      );
    group['weighted-sticky'] = {
      'idle-timeout': 300,
      'max-bindings': 10000,
      ...options,
      nodes,
      resilience: {
        ...resilience,
        enabled: true,
        control: {
          timeout: 3,
          'min-interval': 30,
          'grace-interval': 300,
          ...object(resilience.control),
          'base-url': context.baseURL,
          uuid: context.uuid,
        },
      },
    };
  }
  return config;
}
