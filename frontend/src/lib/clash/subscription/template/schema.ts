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
    nodes: {
      type: 'array',
      minItems: 1,
      maxItems: 32,
      title: '服务器及所属代理',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['name', 'id', 'weight', 'proxies'],
        properties: {
          name: { type: 'string', minLength: 1 },
          id: { type: 'string', minLength: 1 },
          weight: { type: 'integer', minimum: 1 },
          proxies: { ...strings, minItems: 1, uniqueItems: true },
        },
      },
    },
    'max-bindings': { type: 'integer', minimum: 1, title: '最大绑定数' },
    'site-groups': {
      type: 'array',
      items: {
        type: 'object',
        additionalProperties: false,
        required: ['name'],
        properties: {
          name: { type: 'string', minLength: 1 },
          domains: strings,
          'domain-suffixes': strings,
        },
        anyOf: [
          { required: ['domains'], properties: { domains: { ...strings, minItems: 1 } } },
          {
            required: ['domain-suffixes'],
            properties: { 'domain-suffixes': { ...strings, minItems: 1 } },
          },
        ],
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
        required: ['group', 'rules'],
        additionalProperties: false,
        properties: {
          group: { type: 'string', minLength: 1 },
          rules: { ...strings, minItems: 1 },
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
