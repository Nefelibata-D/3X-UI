import { HttpUtil } from '@/utils';
import type { Msg } from '@/utils';
import { parseMsg } from '@/utils/zodValidate';
import {
  ClientRatioViewSchema,
  OverviewSchema,
  SyncStatusSchema,
  type ClientRatioView,
  type ClashOverview,
  type SyncStatus,
  type RatioMode,
} from '@/schemas/clash/allocation';

const OVERVIEW = '/panel/api/clash/overview';
const WEIGHTS = '/panel/api/clash/weights';
const SYNC = '/panel/api/clash/sync';
const RESET = '/panel/api/clash/policies/reset';
const JSON_OPTIONS = { silent: true, headers: { 'Content-Type': 'application/json' } };

function requireObj<T>(msg: Msg<T>, context: string): T {
  if (!msg.success || msg.obj == null) {
    throw new Error(msg.msg || `${context} failed`);
  }
  return msg.obj;
}

export async function fetchClashOverview(): Promise<ClashOverview> {
  const msg = await HttpUtil.get(OVERVIEW, undefined, { silent: true });
  return requireObj(parseMsg(msg, OverviewSchema, 'clash/overview', { strict: true }), 'clash/overview');
}

export interface NodeWeightItem {
  capacityBytes: number;
  resetSchedule: { day: number; minute: number; disabled: boolean };
  defaultWeight: number;
  nodeKey: string;
}

export async function saveClashWeights(items: NodeWeightItem[]): Promise<ClashOverview> {
  const msg = await HttpUtil.post(WEIGHTS, { items }, JSON_OPTIONS);
  return requireObj(parseMsg(msg, OverviewSchema, 'clash/weights', { strict: true }), 'clash/weights');
}

export async function fetchClashClientRatio(clientId: number): Promise<ClientRatioView> {
  const msg = await HttpUtil.get(`/panel/api/clash/clients/${clientId}`, undefined, {
    silent: true,
  });
  return requireObj(parseMsg(msg, ClientRatioViewSchema, 'clash/clients', { strict: true }), 'clash/clients');
}

export async function saveClashClientRatio(
  clientId: number,
  mode: RatioMode,
  weights: Record<string, number>,
  limits: Record<string, number>,
  resetMode: 'node' | 'manual',
): Promise<ClientRatioView> {
  const msg = await HttpUtil.post(
    `/panel/api/clash/clients/${clientId}/ratio`,
    { mode, weights, limits, resetMode },
    JSON_OPTIONS,
  );
  return requireObj(
    parseMsg(msg, ClientRatioViewSchema, 'clash/clients/ratio', { strict: true }),
    'clash/clients/ratio',
  );
}

export async function resetClashPolicy(nodeKey: string, clientId: number): Promise<void> {
  const msg = await HttpUtil.post(RESET, { nodeKey, clientId }, JSON_OPTIONS);
  if (!msg.success) throw new Error(msg.msg || 'clash/reset failed');
}

export async function syncClashPolicies(): Promise<SyncStatus> {
  const msg = await HttpUtil.post(SYNC, undefined, { silent: true });
  const parsed = parseMsg(msg, SyncStatusSchema, 'clash/sync', { strict: true });
  if (!parsed.success) throw new Error(parsed.msg || 'clash/sync failed');
  return requireObj(parsed, 'clash/sync');
}

export function gbToBytes(gb: number | null | undefined): number {
  if (!gb || !Number.isFinite(gb) || gb <= 0) return 0;
  return Math.round(gb * 1024 * 1024 * 1024);
}

export function bytesToGB(bytes: number | null | undefined): number {
  if (!bytes || !Number.isFinite(bytes) || bytes <= 0) return 0;
  return Math.round((bytes / (1024 * 1024 * 1024)) * 100) / 100;
}

export function normalizePreview(
  entries: { key: string; weight: number }[],
): { key: string; ratio: number }[] {
  let sum = 0;
  for (const entry of entries) {
    if (!Number.isFinite(entry.weight) || entry.weight < 0) return [];
    sum += entry.weight;
  }
  if (sum <= 0) return [];
  return entries.map((entry) => ({
    key: entry.key,
    ratio: entry.weight > 0 ? entry.weight / sum : 0,
  }));
}
