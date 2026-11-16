import { Button } from 'antd';
import { StopOutlined, UndoOutlined, DeleteOutlined } from '@ant-design/icons';
import { stringify } from 'yaml';
import type { ComponentProps } from 'react';
import { sameConfigValue } from '@/lib/clash/subscription/publication';
import TemplateInsert from './TemplateInsert';
import NodeGroups from './NodeGroups';
import LoadBalance from './LoadBalance';
import type { Subscription, RuleSet } from '@/schemas/clash/subscription';
import {
  disabledField,
  documentObject,
  entries,
  ruleIdentity,
  setEntries,
  subscriptionFromYAML,
  type Section,
} from '@/lib/clash/subscription/model';

type Group = Record<string, unknown>;
type Blacklist = { group: string; rules: unknown[] };
type BalanceSettings = {
  options: Group;
  blacklist: Blacklist[];
  custom: boolean;
  groupOverride?: boolean;
  positions?: number[];
};
interface Props {
  section: Section;
  balance: boolean;
  base: string;
  binding: Subscription;
  combined: Record<string, unknown>;
  groups: { name: string; type: string }[];
  rules: RuleSet[];
  inbounds: ComponentProps<typeof NodeGroups>['inboundOptions'];
  memberContent: string;
  availableNodes: Record<string, string[]>;
  managedProxyNames: string[];
  onRemove: (name: string) => void;
  showTemplate: boolean;
  showCustom: boolean;
  change: (next: Subscription) => void;
}
const balanceFields = ['type', 'strategy', 'weighted-sticky'] as const;
const blacklists = (config: Group) => (config['site-blacklist'] ?? []) as Blacklist[];
export default function SubscriptionEntries({
  section,
  balance,
  base,
  binding,
  combined,
  groups,
  rules,
  inbounds,
  memberContent,
  availableNodes,
  managedProxyNames,
  onRemove,
  showTemplate,
  showCustom,
  change,
}: Props) {
  const field = disabledField[section];
  const order =
    section === 'rules' ? 'ruleOrder' : section === 'proxy-groups' ? 'groupOrder' : 'providerOrder';
  const identity = (name: string) => (section === 'rules' ? ruleIdentity(name) : name);
  const overlay = documentObject(binding.content),
    paused = documentObject(binding.pausedContent || '{}'),
    template = documentObject(base);
  const custom = new Map(entries(overlay, section)),
    dormant = new Map(entries(paused, section)),
    inherited = new Map(entries(template, section));
  const active = entries(combined, section),
    activeIds = new Set(active.map(([id]) => id));
  const snapshots = { ...(paused.balanceSettings ?? {}) } as Record<string, BalanceSettings>;
  const injections = { ...(paused.groupInjections ?? {}) } as Record<
    string,
    Subscription['groups'][number]
  >;
  function isCustom(id: string) {
    if (custom.has(id)) return true;
    if (section !== 'proxy-groups') return false;
    if (binding.groups.some((row) => row.group === id)) return true;
    return (
      balance &&
      stringify(blacklists(combined).filter((row) => row.group === id)) !==
        stringify(blacklists(template).filter((row) => row.group === id))
    );
  }
  const visible = (name: string) => (isCustom(identity(name)) ? showCustom : showTemplate);
  const entryClassName = (name: string) =>
    isCustom(identity(name)) ? 'subscription-custom-row' : '';
  function toggle(id: string, enable: boolean) {
    const next = { ...binding };
    if (enable) {
      if (dormant.has(id)) custom.set(id, dormant.get(id));
      dormant.delete(id);
      next[field] = binding[field].filter((key) => key !== id);
      if (section === 'proxy-groups' && injections[id]) {
        next.groups = [...binding.groups.filter((row) => row.group !== id), injections[id]];
        delete injections[id];
      }
    } else {
      if (custom.has(id)) dormant.set(id, custom.get(id));
      custom.delete(id);
      next[field] = [...new Set([...binding[field], id])];
      if (!next[order]?.length) next[order] = active.map(([key]) => key);
      if (section === 'proxy-groups') {
        const row = binding.groups.find((row) => row.group === id);
        if (row) injections[id] = row;
        next.groups = binding.groups.filter((row) => row.group !== id);
      }
    }
    next.content = stringify(setEntries(overlay, section, [...custom]));
    next.pausedContent = stringify(
      setEntries({ ...paused, groupInjections: injections }, section, [...dormant]),
    );
    change(next);
  }
  function toggleBalance(id: string, enable: boolean) {
    const current = new Map(active).get(id) as Group;
    const updated = { ...current };
    const others = blacklists(combined).filter((row) => row.group !== id);
    const saved = snapshots[id];
    if (enable) {
      const original = saved?.options ?? (inherited.get(id) as Group);
      for (const key of balanceFields) if (key in original) updated[key] = original[key];
      const restored = saved?.blacklist ?? blacklists(template).filter((row) => row.group === id);
      restored.forEach((row, i) => others.splice(saved?.positions?.[i] ?? others.length, 0, row));
      delete snapshots[id];
    } else {
      snapshots[id] = {
        options: Object.fromEntries(
          balanceFields.filter((k) => k in current).map((k) => [k, current[k]]),
        ),
        blacklist: blacklists(combined).filter((row) => row.group === id),
        custom: isCustom(id),
        groupOverride: custom.has(id),
        positions: blacklists(combined).flatMap((row, i) => (row.group === id ? [i] : [])),
      };
      updated.type = 'select';
      delete updated.strategy;
      delete updated['weighted-sticky'];
    }
    custom.set(id, updated);
    if (
      enable &&
      !(saved?.groupOverride ?? saved?.custom) &&
      sameConfigValue(updated, inherited.get(id))
    )
      custom.delete(id);
    const config = setEntries(overlay, section, [...custom]);
    config['site-blacklist'] = others;
    if (!Object.hasOwn(overlay, 'site-blacklist') || enable) {
      if (sameConfigValue(others, blacklists(template))) delete config['site-blacklist'];
    }
    change({
      ...binding,
      content: stringify(config),
      pausedContent: stringify({ ...paused, balanceSettings: snapshots }),
    });
  }
  const disabled = balance
    ? active
        .filter(
          ([id, value]) =>
            (value as Group).type !== 'load-balance' &&
            (snapshots[id] || (inherited.get(id) as Group)?.type === 'load-balance'),
        )
        .map(([id]) => id)
    : [...new Set([...binding[field], ...dormant.keys()])].filter(
        (id) => !activeIds.has(id) && (dormant.has(id) || inherited.has(id)),
      );
  const beforeAdd = disabled
    .filter((id) =>
      (balance ? snapshots[id]?.custom : dormant.has(id) || injections[id])
        ? showCustom
        : showTemplate,
    )
    .map((id) => (
      <div className="subscription-disabled-row" key={id}>
        <StopOutlined aria-label="已禁用" />
        <span>{section === 'rules' ? String(dormant.get(id) ?? inherited.get(id)) : id}</span>
        <Button
          type="text"
          size="small"
          icon={<UndoOutlined />}
          aria-label={'恢复 ' + id}
          onClick={() => (balance ? toggleBalance(id, true) : toggle(id, true))}
        >
          恢复
        </Button>
      </div>
    ));
  function edited(content: string) {
    const next = subscriptionFromYAML(base, content, binding);
    const ids = entries(documentObject(content), section).map(([id]) => id);
    if (ids.some((id) => dormant.has(id) && !activeIds.has(id)))
      throw Error('该条目已禁用，请先恢复');
    if (ids.length > active.length) {
      const added = ids.filter((id) => !activeIds.has(id));
      if (added.length !== ids.length - active.length) throw Error('条目重复');
      next[order] = [...added, ...ids.filter((id) => activeIds.has(id))];
    }
    return next;
  }
  function rowActions(name: string) {
    const id = identity(name);
    const origin = inherited.get(id) as Group | undefined;
    const preset = balance ? origin?.type === 'load-balance' : inherited.has(id);
    function resetOrDelete() {
      if (balance) {
        const group = { ...(new Map(active).get(id) as Group) };
        for (const key of balanceFields) {
          if (preset && origin && key in origin) group[key] = origin[key];
          else delete group[key];
        }
        if (!preset) group.type = origin?.type ?? 'select';
        if (sameConfigValue(group, origin)) custom.delete(id);
        else custom.set(id, group);
        const config = setEntries(overlay, section, [...custom]);
        const rows = blacklists(combined).filter((row) => row.group !== id);
        const originalRows = blacklists(template);
        if (preset)
          originalRows.forEach((row, i) => {
            if (row.group === id) rows.splice(i, 0, row);
          });
        if (sameConfigValue(rows, originalRows)) delete config['site-blacklist'];
        else config['site-blacklist'] = rows;
        delete snapshots[id];
        change({
          ...binding,
          content: stringify(config),
          pausedContent: stringify({ ...paused, balanceSettings: snapshots }),
        });
        return;
      }
      if (!preset && section === 'proxy-groups') {
        onRemove(id);
        return;
      }
      custom.delete(id);
      change({
        ...binding,
        content: stringify(setEntries(overlay, section, [...custom])),
        groups:
          section === 'proxy-groups'
            ? binding.groups.filter((g) => g.group !== id)
            : binding.groups,
      });
    }
    const action = preset ? '重置' : '删除';
    const resetDelete = (
      <Button
        key="reset-delete"
        type="text"
        danger={!preset}
        title={action}
        aria-label={action + ' ' + id}
        disabled={preset && !isCustom(id)}
        icon={preset ? <UndoOutlined /> : <DeleteOutlined />}
        onClick={resetOrDelete}
      />
    );
    const disable = (
      <Button
        key="disable"
        type="text"
        danger
        title="禁用"
        aria-label={'禁用 ' + id}
        icon={<StopOutlined />}
        onClick={() => (balance ? toggleBalance(id, false) : toggle(id, false))}
      />
    );
    return preset ? [disable, resetDelete] : [resetDelete, disable];
  }
  const content = stringify(combined);
  const common = { content, readonly: false, beforeAdd, entryClassName, rowActions };
  return (
    <div className="subscription-route-panel">
      <div className="subscription-route-scroll">
        {balance ? (
          <LoadBalance
            {...common}
            rules={rules}
            availableNodes={availableNodes}
            managedProxyNames={managedProxyNames}
            visibleGroups={groups.filter((g) => visible(g.name)).map((g) => g.name)}
            onDisable={(id) => toggleBalance(id, false)}
            onChange={(value) => change(edited(value))}
          />
        ) : section === 'proxy-groups' ? (
          <NodeGroups
            {...common}
            visible={visible}
            groups={groups}
            effectiveContent={memberContent}
            bindings={binding.groups}
            inboundOptions={inbounds}
            invalid={false}
            onRemove={onRemove}
            entryClassName={(name) => 'subscription-node-card ' + entryClassName(name)}
            change={(value, bindings) => change({ ...edited(value), groups: bindings })}
          />
        ) : (
          <TemplateInsert
            {...common}
            tab={section === 'rules' ? 'routes' : 'providers'}
            groups={groups.map((g) => g.name)}
            rules={rules}
            visible={visible}
            onDisable={(name) => toggle(identity(name), false)}
            onChange={(value) => change(edited(value))}
          />
        )}
      </div>
    </div>
  );
}
