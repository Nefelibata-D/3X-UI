import upstream from 'meta-json-schema/schemas/meta-json-schema.json';
import type { JSONSchema } from 'monaco-yaml';

export const templateSchema = structuredClone(upstream) as unknown as JSONSchema;
const groups = templateSchema.definitions!['proxy-groups'];
const loadBalance = groups.definitions!['load-balance'];
const groupType = loadBalance.definitions!.groupbase.definitions!['base-option'].properties!
  .type as JSONSchema;
groupType.enum = ['select', 'url-test', 'fallback', 'load-balance'];
delete groupType.markdownEnumDescriptions;
const items = groups.items as JSONSchema;
items.anyOf = items.anyOf!.slice(0, 4);
const balanceProperties = (loadBalance.allOf![1] as JSONSchema).properties!;
const strategy = balanceProperties.strategy as JSONSchema;
strategy.enum = ['consistent-hashing', 'round-robin', 'sticky-sessions', 'weighted-sticky'];
delete strategy.markdownEnumDescriptions;
const strings: JSONSchema = { type: 'array', items: { type: 'string' } };
balanceProperties['weighted-sticky'] = {
  type: 'object',
  additionalProperties: false,
  title: '按权重保持节点绑定',
  properties: {
    'idle-timeout': { type: 'integer', minimum: 1, maximum: 9223372036, title: '空闲超时（秒）' },
    'default-weight': { type: 'integer', minimum: 0, title: '默认权重' },
    weights: {
      type: 'object',
      additionalProperties: { type: 'integer', minimum: 0 },
      title: '代理名称与权重',
    },
    'max-bindings': { type: 'integer', minimum: 1, title: '最大绑定数' },
    'site-groups': {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['name'],
        properties: { name: { type: 'string' }, domains: strings, 'domain-suffixes': strings },
      },
    },
    resilience: {
      type: 'object',
      additionalProperties: false,
      properties: {
        enabled: { type: 'boolean' },
        control: {
          type: 'object',
          additionalProperties: false,
          properties: {
            'base-url': { type: 'string' },
            uuid: { type: 'string' },
            timeout: { type: 'integer', minimum: 1, maximum: 9223372036 },
            'min-interval': { type: 'integer', minimum: 1, maximum: 9223372036 },
            'grace-interval': { type: 'integer', minimum: 1, maximum: 9223372036 },
            'node-ids': { type: 'object', additionalProperties: { type: 'string' } },
          },
        },
      },
    },
  },
};
templateSchema.allOf!.push({
  type: 'object',
  properties: {
    'site-blacklist': {
      type: 'array',
      title: '新绑定时避开的节点',
      items: {
        type: 'object',
        required: ['proxy', 'rules'],
        additionalProperties: false,
        properties: {
          group: { type: 'string', minLength: 1 },
          proxy: { type: 'string' },
          rules: strings,
        },
      },
    },
  },
});

export const proxyTypes = (templateSchema.definitions!.proxies.items as JSONSchema).anyOf!.map(
  (option) =>
    String(
      ((option as JSONSchema).allOf![0] as JSONSchema).properties!.type &&
        (((option as JSONSchema).allOf![0] as JSONSchema).properties!.type as JSONSchema).const,
    ),
);
