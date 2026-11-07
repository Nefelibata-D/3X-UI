import { z } from 'zod';

export const RatioModeSchema = z.enum(['inherit', 'custom']);
export type RatioMode = z.infer<typeof RatioModeSchema>;

export const ResetScheduleSchema = z.object({
  day: z.number().int().min(0).max(31),
  minute: z.number().int().min(0).max(1439),
  disabled: z.boolean(),
});

export const ManagedInboundSchema = z.object({
  id: z.number(),
  tag: z.string(),
  protocol: z.string().optional().default(''),
  listen: z.string().optional().default(''),
  port: z.number().optional().default(0),
  network: z.string().optional().default(''),
  security: z.string().optional().default(''),
  enabled: z.boolean(),
});

export const PolicyOverviewSchema = z.object({
  masterGuid: z.string().optional().default(''),
  nodeKey: z.string(),
  nodeName: z.string().optional().default(''),
  clientId: z.number(),
  clientEmail: z.string().optional().default(''),
  revision: z.number().optional().default(0),
  status: z.string().optional().default(''),
  lastError: z.string().optional().default(''),
  lastReportAt: z.number().optional().default(0),
  resetPending: z.boolean().optional().default(false),
  remove: z.boolean().optional().default(false),
  state: z.string().optional().default(''),
  budgetBytes: z.number().optional().default(0),
  defaultWeight: z.number().optional().default(1),
  effectiveRatio: z.number().optional().default(0),
  usageUp: z.number().optional().default(0),
  usageDown: z.number().optional().default(0),
  periodStart: z.number().optional().default(0),
  periodEnd: z.number().optional().default(0),
  graceStartedAt: z.number().optional().default(0),
  graceDeadline: z.number().optional().default(0),
  graceExtraBytes: z.number().optional().default(0),
  gatewayPort: z.number().optional().default(0),
  gatewayListening: z.boolean().optional().default(false),
  activeConnections: z.number().optional().default(0),
  managedInbounds: z
    .array(ManagedInboundSchema)
    .nullish()
    .transform((v) => v ?? []),
  applied: z.boolean().optional().default(false),
  applyError: z.string().optional().default(''),
  disabledAt: z.number().optional().default(0),
  remote: z.boolean().optional().default(false),
});

export const NodeOverviewSchema = z.object({
  capacityBytes: z.number(),
  allocatedBytes: z.number(),
  resetSchedule: ResetScheduleSchema,
  key: z.string(),
  id: z.number(),
  name: z.string(),
  guid: z.string().optional().default(''),
  enabled: z.boolean(),
  status: z.string().optional().default(''),
  isLocal: z.boolean().optional().default(false),
  defaultWeight: z.number().optional().default(1),
  defaultRatio: z.number().optional().default(0),
});

export const SyncStatusSchema = z.object({
  status: z.string().optional().default('idle'),
  error: z.string().optional().default(''),
  lastSync: z.number().optional().default(0),
  running: z.boolean().optional().default(false),
});

export const OverviewSchema = z.object({
  panelGuid: z.string().optional().default(''),
  timezone: z.string().optional().default(''),
  generatedAt: z.number().optional().default(0),
  nodes: z
    .array(NodeOverviewSchema)
    .nullish()
    .transform((v) => v ?? []),
  policies: z
    .array(PolicyOverviewSchema)
    .nullish()
    .transform((v) => v ?? []),
  sync: SyncStatusSchema.optional().default({
    status: 'idle',
    error: '',
    lastSync: 0,
    running: false,
  }),
});

export const ClientNodeViewSchema = z.object({
  capacityBytes: z.number(),
  allocatedBytes: z.number(),
  resetSchedule: ResetScheduleSchema,
  nodeKey: z.string(),
  name: z.string(),
  guid: z.string().optional().default(''),
  enabled: z.boolean(),
  budgetBytes: z.number().optional().default(0),
  defaultWeight: z.number().optional().default(1),
  selectable: z.boolean(),
  usageUp: z.number(),
  usageDown: z.number(),
  usageKnown: z.boolean(),
  periodEnd: z.number(),
  applied: z.boolean(),
  applyError: z.string(),
  state: z.string(),
  defaultRatio: z.number().optional().default(0),
  overrideWeight: z.number().optional().default(0),
  effectiveRatio: z.number().optional().default(0),
});

export const ClientRatioViewSchema = z.object({
  resetMode: z.enum(['node', 'manual']),
  timezone: z.string(),
  totalBytes: z.number(),
  clientId: z.number(),
  email: z.string().optional().default(''),
  mode: RatioModeSchema,
  weightError: z.string().optional().default(''),
  nodes: z
    .array(ClientNodeViewSchema)
    .nullish()
    .transform((v) => v ?? []),
});

export type ManagedInbound = z.infer<typeof ManagedInboundSchema>;
export type PolicyOverview = z.infer<typeof PolicyOverviewSchema>;
export type NodeOverview = z.infer<typeof NodeOverviewSchema>;
export type SyncStatus = z.infer<typeof SyncStatusSchema>;
export type ClashOverview = z.infer<typeof OverviewSchema>;
export type ClientNodeView = z.infer<typeof ClientNodeViewSchema>;
export type ClientRatioView = z.infer<typeof ClientRatioViewSchema>;
