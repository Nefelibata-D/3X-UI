import { z } from 'zod';
import { HttpUtil, type Msg } from '@/utils';
import {
  ruleSetSchema,
  templateSchema,
  subscriptionSchema,
  subscriptionProxySchema,
  type RuleSet,
  type Template,
  type Subscription,
} from '@/schemas/clash/subscription';

const root = '/panel/api/clash';
function clientPath(id: number | undefined) {
  if (id === undefined || !Number.isInteger(id) || id <= 0) throw new Error('客户端没有有效 ID');
  return `/clients/${id}/subscription`;
}
const json = { silent: true, headers: { 'Content-Type': 'application/json' } };
function success<T>(response: Msg<T>): T | null {
  if (!response.success) throw new Error(response.msg);
  return response.obj;
}
async function get<T extends z.ZodType>(path: string, schema: T): Promise<z.infer<T>> {
  return schema.parse(success(await HttpUtil.get(root + path, undefined, { silent: true })));
}
export const fetchRuleSets = () =>
  get(
    '/rulesets',
    ruleSetSchema
      .array()
      .nullable()
      .transform((rows) => rows ?? []),
  );
export const fetchTemplates = () =>
  get(
    '/templates',
    templateSchema
      .array()
      .nullable()
      .transform((rows) => rows ?? []),
  );
export const fetchSubscription = (id: number | undefined) =>
  get(clientPath(id), subscriptionSchema);
export const fetchSubscriptionProxies = (id: number | undefined) =>
  get(clientPath(id) + '/proxies', subscriptionProxySchema.array());
export async function saveRuleSet(row: RuleSet) {
  success(await HttpUtil.post(root + '/rulesets', ruleSetSchema.parse(row), json));
}
export async function deleteRuleSet(id: number) {
  success(await HttpUtil.delete(root + `/rulesets/${id}`, { silent: true }));
}
export async function importRules(content: string) {
  return z
    .array(z.string())
    .parse(success(await HttpUtil.post(root + '/rulesets/import', { content }, json)));
}
export async function saveTemplate(row: Template) {
  success(await HttpUtil.post(root + '/templates', templateSchema.parse(row), json));
}
export async function deleteTemplate(id: number) {
  success(await HttpUtil.delete(root + `/templates/${id}`, { silent: true }));
}
export async function saveSubscription(id: number | undefined, row: Subscription) {
  success(await HttpUtil.post(root + clientPath(id), subscriptionSchema.parse(row), json));
}
export async function validateSubscription(id: number | undefined, row: Subscription) {
  success(
    await HttpUtil.post(root + clientPath(id) + '/validate', subscriptionSchema.parse(row), json),
  );
}
