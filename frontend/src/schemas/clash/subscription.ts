import { z } from 'zod';

const strings = z
  .array(z.string())
  .nullish()
  .transform((v) => v ?? []);
export const subscriptionSchema = z.object({
  lastSyncAt: z.number().optional().default(0),
  updatedAt: z.number().optional().default(0),
  templateId: z.number(),
  dialers: z
    .record(z.string(), z.string())
    .nullish()
    .transform((v) => v ?? {}),
  disabledRules: strings,
  disabledProviders: strings,
  disabledGroups: strings,
  content: z.string(),
  groups: z
    .array(z.object({ group: z.string(), allNodes: z.boolean(), nodes: strings }))
    .nullish()
    .transform((v) => v ?? []),
});
export type Subscription = z.infer<typeof subscriptionSchema>;
export const emptySubscription: Subscription = {
  updatedAt: 0,
  lastSyncAt: 0,
  templateId: 0,
  dialers: {},
  disabledRules: [],
  disabledProviders: [],
  disabledGroups: [],
  content: '{}',
  groups: [],
};
const injectionsSchema = z.object({
  groups: z
    .array(
      z.object({
        group: z.string(),
        allNodes: z.boolean(),
        nodes: z
          .array(z.string())
          .nullable()
          .transform((v) => v ?? []),
      }),
    )
    .nullable()
    .transform((v) => v ?? []),
  routes: z
    .array(z.object({ ruleSetId: z.number(), target: z.string() }))
    .nullable()
    .transform((v) => v ?? []),
  blacklists: z
    .array(z.object({ ruleSetId: z.number(), node: z.string() }))
    .nullable()
    .transform((v) => v ?? []),
});
export const templateSchema = z.object({
  id: z.number(),
  name: z.string(),
  content: z.string(),
  groups: z.array(z.string()),
  injections: injectionsSchema
    .nullish()
    .transform((v) => v ?? { groups: [], routes: [], blacklists: [] }),
});
export type Template = z.infer<typeof templateSchema>;

export const ruleSetSchema = z.object({
  updatedAt: z.string().nullable().optional(),
  id: z.number(),
  code: z.string(),
  filename: z.string(),
  name: z.string(),
  routing: z.boolean(),
  blacklist: z.boolean(),
  rules: z.array(z.string()),
});
export type RuleSet = z.infer<typeof ruleSetSchema>;
export const subscriptionProxySchema = z.object({
  name: z.string(),
  type: z.string(),
  nodeKey: z.string(),
  inboundKey: z.string(),
  inboundName: z.string(),
  hostOverride: z.boolean(),
  config: z.record(z.string(), z.unknown()),
});
export type SubscriptionProxy = z.infer<typeof subscriptionProxySchema>;
