import SortableTemplateRow, {
  SortableTemplateList,
} from '@/components/clash/subscription/SortableTemplateRow';
import { useId, useState } from 'react';
import type { ReactNode } from 'react';
import { Alert, Button, Input, Select, Space, Tag } from 'antd';
import { CheckOutlined, DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { isMap, isSeq, parseDocument } from 'yaml';

import {
  dependencyCycle,
  templateDependencies,
} from '@/lib/clash/subscription/template/validation';
import {
  builtins,
  groupTypes,
  replaceSymbol,
  templateSymbols,
} from '@/lib/clash/subscription/template/symbols';
type Binding = { group: string; allNodes: boolean; nodes: string[] };
interface Props {
  inboundOptions?: { inboundKey: string; name: string; hostOverride: boolean }[];
  onRemove?: (name: string) => void;
  entryActions?: (name: string) => ReactNode;
  entryClassName?: (name: string) => string;
  content: string;
  referenceContent?: string;
  effectiveContent?: string;
  groups: { name: string; type: string }[];
  bindings: Binding[];
  readonly: boolean;
  invalid: boolean;
  change: (content: string, bindings: Binding[], removed?: string, replacement?: string) => void;
}
export default function NodeGroups({
  content,
  referenceContent = content,
  effectiveContent = content,
  groups,
  bindings,
  readonly,
  invalid,
  change,
  entryActions,
  entryClassName,
  onRemove,
  inboundOptions,
}: Props) {
  const uid = useId();
  const [editError, setEditError] = useState('');
  const effectiveDependencies = templateDependencies(effectiveContent).nodes;
  const referenceNodes = templateDependencies(referenceContent).nodes;
  const [draft, setDraft] = useState<{
    original: string;
    key?: string;
    name: string;
    type: string;
    members: string[];
    nodes: string[];
    allNodes: boolean;
    remove: boolean;
    replacement: string;
  }>();
  function open(name?: string, remove = false) {
    setEditError('');
    const list = parseDocument(content).get('proxy-groups');
    const group = isSeq(list)
      ? list.items.find((g) => isMap(g) && g.get('name') === name)
      : undefined;
    const members = isMap(group) ? group.get('proxies') : undefined;
    setDraft({
      nodes: bindings.find((b) => b.group === name)?.nodes ?? [],
      allNodes: bindings.find((b) => b.group === name)?.allNodes ?? false,
      members: isSeq(members) ? members.items.map(String) : [],
      original: content,
      key: name,
      name: name ?? '',
      type: groups.find((g) => g.name === name)?.type ?? 'select',
      remove,
      replacement: 'DIRECT',
    });
  }
  function apply() {
    if (!draft) return;
    try {
      if (content !== draft.original) throw Error('基础 YAML 已变更，请取消后重新打开。');
      const doc = parseDocument(content);
      if (doc.errors.length) throw doc.errors[0];
      if (!doc.has('proxy-groups')) doc.set('proxy-groups', doc.createNode([]));
      const seq = doc.get('proxy-groups');
      if (!isSeq(seq)) throw Error('proxy-groups 必须是列表');
      const name = draft.name.trim();
      const proxies = doc.get('proxies');
      if (
        !draft.remove &&
        (!name ||
          name.includes(',') ||
          builtins.includes(name) ||
          groups.some((g) => g.name === name && name !== draft.key) ||
          (isSeq(proxies) && proxies.items.some((p) => isMap(p) && p.get('name') === name)))
      )
        throw Error('请使用唯一的代理组名称');
      const replacement = draft.remove ? draft.replacement : name;
      if (draft.key) {
        const idx = seq.items.findIndex((g) => isMap(g) && g.get('name') === draft.key);
        if (idx < 0) throw Error('代理组不存在');
        if (draft.remove) seq.delete(idx);
        else doc.setIn(['proxy-groups', idx, 'proxies'], draft.members);
        if (draft.key !== replacement)
          replaceSymbol(templateSymbols(doc), 'proxy', draft.key, replacement);
      } else {
        seq.add(
          doc.createNode({
            name,
            type: draft.type,
            proxies: draft.members,
            ...(draft.type === 'load-balance' ? { strategy: 'weighted-sticky' } : {}),
          }),
        );
      }
      const effective = parseDocument(referenceContent);
      if (!effective.has('proxy-groups')) effective.set('proxy-groups', effective.createNode([]));
      const effectiveGroups = effective.get('proxy-groups');
      if (!isSeq(effectiveGroups)) throw Error('proxy-groups 必须是列表');
      const index = effectiveGroups.items.findIndex((g) => isMap(g) && g.get('name') === draft.key);
      const edited = seq.items.find((g) => isMap(g) && g.get('name') === name);
      if (draft.remove) {
        if (index >= 0) effectiveGroups.delete(index);
      } else if (isMap(edited)) {
        if (index >= 0) effectiveGroups.set(index, edited.clone());
        else effectiveGroups.add(edited.clone());
      }
      if (draft.key && draft.key !== replacement)
        replaceSymbol(templateSymbols(effective), 'proxy', draft.key, replacement);
      const cycle = dependencyCycle(templateDependencies(effective.toString()).nodes);
      if (cycle) throw Error('循环引用：' + cycle.join(' → '));
      let next = draft.remove
        ? bindings.filter((b) => b.group !== draft.key)
        : bindings.map((b) => (b.group === draft.key ? { ...b, group: name } : b));
      if (inboundOptions && !draft.remove)
        next = [
          ...next.filter((b) => b.group !== name),
          { group: name, nodes: draft.nodes, allNodes: draft.allNodes },
        ];
      change(doc.toString(), next, draft.key, replacement);
      setDraft(undefined);
    } catch (e) {
      setEditError(e instanceof Error ? e.message : String(e));
    }
  }
  const groupNames = new Set(
    referenceNodes.filter((node) => node.kind === 'group').map((node) => node.name),
  );
  const memberNames = [
    ...new Set([
      ...referenceNodes
        .filter((node) => node.kind !== 'group' || node.name !== draft?.key)
        .map((node) => node.name),
      ...(draft?.members ?? []),
    ]),
  ].filter((name) => !builtins.includes(name));
  function renderMember(label: ReactNode, value: unknown, option = false) {
    const [kind, key] = JSON.parse(String(value)) as [string, string];
    const inbound =
      kind === 'node' ? inboundOptions?.find((item) => item.inboundKey === key) : undefined;
    const group = kind === 'member' && groupNames.has(key);
    const system = kind === 'node' || builtins.includes(key);
    return (
      <span
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          gap: 8,
          width: option ? '100%' : undefined,
        }}
      >
        <span>{label}</span>
        {kind !== 'all' && (
          <Space size={4}>
            <Tag color={group ? 'purple' : system ? 'cyan' : 'gold'}>
              {group ? '节点组' : system ? '系统出站' : '预设出站'}
            </Tag>
            {inbound?.hostOverride && <Tag color="blue">主机覆盖</Tag>}
          </Space>
        )}
      </span>
    );
  }
  const editor = draft && (
    <div className="template-entry-edit node-group-fields">
      {draft.remove ? (
        <>
          <strong>移除「{draft.name}」？</strong>
          <label htmlFor={uid + '-replacement'}>
            引用改为
            <Select
              id={uid + '-replacement'}
              value={draft.replacement}
              onChange={(replacement) => setDraft({ ...draft, replacement })}
              options={['DIRECT', 'REJECT'].map((value) => ({ value }))}
            />
          </label>
        </>
      ) : (
        <>
          <label htmlFor={uid + '-name'}>
            组名
            <Input
              id={uid + '-name'}
              value={draft.name}
              placeholder="代理组名称"
              onChange={(e) => setDraft({ ...draft, name: e.target.value })}
            />
          </label>
          <label htmlFor={uid + '-members'}>
            代理
            <Select
              id={uid + '-members'}
              mode="multiple"
              value={[
                ...draft.members.map((name) => JSON.stringify(['member', name])),
                ...(draft.allNodes
                  ? [JSON.stringify(['all', '*'])]
                  : draft.nodes.map((key) => JSON.stringify(['node', key]))),
              ]}
              showSearch={{ optionFilterProp: 'label' }}
              menuItemSelectedIcon={({ isSelected }) => (
                <CheckOutlined
                  aria-hidden
                  style={{
                    width: 20,
                    marginInlineStart: 4,
                    visibility: isSelected ? 'visible' : 'hidden',
                  }}
                />
              )}
              optionRender={(option) => renderMember(option.label, option.value, true)}
              labelRender={({ label, value }) => renderMember(label ?? value, value)}
              options={[
                ...(inboundOptions
                  ? [{ value: JSON.stringify(['all', '*']), label: '全部已分配代理' }]
                  : []),
                {
                  label: '预设代理',
                  options: builtins.map((name) => ({
                    value: JSON.stringify(['member', name]),
                    label: name,
                  })),
                },
                {
                  label: '自定义代理',
                  options: [
                    ...memberNames
                      .filter((name) => !groupNames.has(name))
                      .map((name) => ({ value: JSON.stringify(['member', name]), label: name })),
                    ...(inboundOptions ?? []).map((inbound) => ({
                      value: JSON.stringify(['node', inbound.inboundKey]),
                      label: inbound.name,
                    })),
                    ...memberNames
                      .filter((name) => groupNames.has(name))
                      .map((name) => ({ value: JSON.stringify(['member', name]), label: name })),
                  ],
                },
              ]}
              onChange={(values: string[]) => {
                const selected = values.map((value) => JSON.parse(value) as [string, string]);
                const allNodes = selected.some(([kind]) => kind === 'all');
                setDraft({
                  ...draft,
                  allNodes,
                  members: selected.filter(([kind]) => kind === 'member').map(([, value]) => value),
                  nodes: allNodes
                    ? []
                    : selected.filter(([kind]) => kind === 'node').map(([, value]) => value),
                });
              }}
              placeholder={inboundOptions ? '选择代理' : '留空，等待客户端注入'}
            />
          </label>
          <label htmlFor={uid + '-type'}>
            类型
            <Select
              id={uid + '-type'}
              disabled={!!draft.key}
              value={draft.type}
              options={groupTypes.map((value) => ({
                value,
              }))}
              onChange={(type) => setDraft({ ...draft, type })}
            />
          </label>
        </>
      )}
      {editError && <Alert type="error" title={editError} />}
      <div className="template-entry-actions">
        <Space>
          <Button onClick={() => setDraft(undefined)}>取消</Button>
          <Button type="primary" danger={draft.remove} disabled={invalid} onClick={apply}>
            {draft.remove ? '移除代理组' : '确定'}
          </Button>
        </Space>
      </div>
    </div>
  );
  return (
    <SortableTemplateList
      content={content}
      collection="proxy-groups"
      enabled={!readonly && !draft && !invalid}
      onChange={(value) => change(value, bindings)}
    >
      {groups.map((group) => {
        return (
          <div
            key={group.name}
            data-sort-item
            className={draft?.key === group.name ? undefined : entryClassName?.(group.name)}
          >
            {draft?.key === group.name ? (
              editor
            ) : (
              <SortableTemplateRow showHandle={!readonly} enabled={!readonly && !draft && !invalid}>
                <div className="node-group-summary">
                  <div className="node-group-heading">
                    <span className="template-entry-title" title={group.name}>
                      {group.name}
                    </span>
                    <Tag color="blue">{group.type.toUpperCase()}</Tag>
                  </div>
                  {effectiveDependencies.find((n) => n.name === group.name)?.pending && (
                    <Tag color="orange">待客户端注入</Tag>
                  )}
                </div>
                {!readonly && (
                  <Space size={4}>
                    <Button
                      type="text"
                      aria-label={'编辑 ' + group.name}
                      className={draft?.key === group.name ? 'template-edit-active' : undefined}
                      aria-pressed={draft?.key === group.name}
                      icon={<EditOutlined aria-hidden />}
                      disabled={!!draft || invalid}
                      onClick={() => open(group.name)}
                    />
                    <Button
                      type="text"
                      danger
                      aria-label={'移除 ' + group.name}
                      icon={<DeleteOutlined aria-hidden />}
                      disabled={!!draft || invalid}
                      onClick={() => (onRemove ? onRemove(group.name) : open(group.name, true))}
                    />
                  </Space>
                )}
                {entryActions?.(group.name)}
              </SortableTemplateRow>
            )}
          </div>
        );
      })}
      {!readonly && !draft && (
        <Button
          className="template-entry-add"
          icon={<PlusOutlined aria-hidden />}
          disabled={invalid}
          onClick={() => open()}
        >
          添加代理组
        </Button>
      )}
      {draft && !draft.key && editor}
    </SortableTemplateList>
  );
}
