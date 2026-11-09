import { parseBlacklistRule, formatBlacklistRule } from '@/lib/clash/subscription/blacklist';
import { useEffect, useId, useState } from 'react';
import type { ReactNode } from 'react';
import { Alert, message, Button, Input, Select, Space, Tag, Tooltip } from 'antd';
import { DeleteOutlined, EditOutlined, EyeOutlined, PlusOutlined } from '@ant-design/icons';
import { isMap, isSeq } from 'yaml';
import {
  blacklistRuleKinds,
  loadBalanceErrors,
  loadBalanceModel,
  weightedGroupIssue,
} from '@/lib/clash/subscription/loadBalance';

interface Props {
  visibleGroups?: string[];
  entryActions?: (name: string) => ReactNode;
  entryClassName?: (name: string) => string;
  availableNodes?: Record<string, string[]>;
  managedProxyNames?: string[];
  content: string;
  readonly: boolean;
  onChange: (value: string) => void;
  rules: { name: string; code: string; filename: string; blacklist: boolean }[];
}
export default function LoadBalance(props: Props) {
  const [editing, setEditing] = useState<{ original: string; content: string; group?: string }>();
  const [pendingRule, setPendingRule] = useState(false);
  if (!editing)
    return (
      <BalanceContent
        {...props}
        onEdit={(group) => {
          setPendingRule(false);
          setEditing({ original: props.content, content: props.content, group });
        }}
      />
    );
  const stale = editing.original !== props.content;
  return (
    <div className="template-entry-edit template-balance-editor">
      {stale && <Alert type="warning" title="基础 YAML 已变更，请取消后重新编辑。" />}
      <BalanceContent
        {...props}
        content={editing.content}
        onChange={(content) => setEditing({ ...editing, content })}
        editing
        initialGroup={editing.group}
        onPendingRule={setPendingRule}
      />
      {!pendingRule && (
        <Space>
          <Button onClick={() => setEditing(undefined)}>{props.readonly ? '返回' : '取消'}</Button>
          {!props.readonly && (
            <Button
              type="primary"
              disabled={stale || pendingRule}
              onClick={() => {
                const errors = loadBalanceErrors(editing.content);
                if (errors.length) {
                  void message.error(errors[0]);
                  return;
                }
                props.onChange(editing.content);
                setEditing(undefined);
              }}
            >
              确认
            </Button>
          )}
        </Space>
      )}
    </div>
  );
}
function BalanceContent({
  content,
  readonly,
  onChange,
  rules,
  editing = false,
  initialGroup,
  availableNodes,
  managedProxyNames,
  visibleGroups,
  onEdit,
  onPendingRule,
  entryActions,
  entryClassName,
}: Props & {
  editing?: boolean;
  initialGroup?: string;
  onEdit?: (group?: string) => void;
  onPendingRule?: (pending: boolean) => void;
}) {
  const uid = useId();
  const [selected, select] = useState<string | undefined>(initialGroup);
  const [expanded, setExpanded] = useState<Set<string>>(() => new Set());
  const [adding, setAdding] = useState(editing && !initialGroup);
  const [newGroup, setNewGroup] = useState<string>();
  const [draft, setDraft] = useState<{
    proxies: string[];
    kind: string;
    value: string;
    original?: string;
  }>();
  useEffect(() => {
    onPendingRule?.(!!draft);
  }, [draft, onPendingRule]);
  const { doc, groups } = loadBalanceModel(content);
  const group = groups.find((g) => g.name === selected);
  const weightedIssue = (name: string) =>
    weightedGroupIssue(
      availableNodes?.[name] ?? groups.find((g) => g.name === name)?.members ?? [],
      groups.map((g) => g.name),
      managedProxyNames,
    );
  const groupOption = (name: string, weighted: boolean) => {
    const reason = weighted ? weightedIssue(name) : '';
    return { value: name, label: name, disabled: !!reason, reason };
  };
  const optionLabel = (label: ReactNode, reason?: string) => (
    <Tooltip title={reason}>
      <span style={{ display: 'block', width: '100%' }}>{label}</span>
    </Tooltip>
  );
  const errors = loadBalanceErrors(content);
  const entries = doc.get('site-blacklist');
  const providers = doc.get('rule-providers');
  const providerNames = isMap(providers)
    ? providers.items
        .filter(
          (p) =>
            isMap(p.value) &&
            p.value.get('behavior') === 'classical' &&
            p.value.get('format') !== 'mrs',
        )
        .map((p) => String(p.key))
    : [];
  const names = [
    ...new Set([...providerNames, ...rules.filter((r) => r.blacklist).map((r) => r.name)]),
  ];
  function commit() {
    const problems = loadBalanceErrors(doc.toString());
    if (problems.length) {
      void message.error(problems[0]);
      return false;
    }
    onChange(doc.toString());
    return true;
  }
  function changeGroup(from: string, to: string) {
    if (from === to) return;
    const source = groups.find((g) => g.name === from);
    const target = groups.find((g) => g.name === to);
    if (!source || !target || target.type === 'load-balance') return;
    if (source.strategy === 'weighted-sticky' && weightedIssue(to)) return;
    for (const key of ['strategy', 'weighted-sticky']) {
      const value = doc.getIn(['proxy-groups', source.index, key], true);
      if (value !== undefined) doc.setIn(['proxy-groups', target.index, key], value);
      else doc.deleteIn(['proxy-groups', target.index, key]);
      doc.deleteIn(['proxy-groups', source.index, key]);
    }
    doc.setIn(['proxy-groups', source.index, 'type'], 'select');
    doc.setIn(['proxy-groups', target.index, 'type'], 'load-balance');
    if (isSeq(entries))
      for (const entry of entries.items) {
        if (isMap(entry) && entry.get('group') === from) entry.set('group', to);
      }
    if (commit()) {
      select(to);
      setDraft(undefined);
    }
  }
  function ruleRows(name: string) {
    const rows = new Map<string, Set<string>>();
    if (isSeq(entries))
      for (const entry of entries.items) {
        if (!isMap(entry) || entry.get('group') !== name) continue;
        const rules = entry.get('rules');
        if (isSeq(rules))
          for (const rule of rules.items) {
            try {
              const parsed = parseBlacklistRule(String(rule));
              if (!rows.has(parsed.rule)) rows.set(parsed.rule, new Set());
              for (const proxy of parsed.proxies) rows.get(parsed.rule)!.add(proxy);
            } catch {
              continue;
            }
          }
      }
    return [...rows].map(([rule, proxies]) => ({ rule, proxies: [...proxies] }));
  }
  function deleteRule(name: string, rule: string, save = true) {
    if (!isSeq(entries)) return;
    for (let i = entries.items.length - 1; i >= 0; i--) {
      const entry = entries.items[i];
      if (!isMap(entry) || entry.get('group') !== name) continue;
      const rules = entry.get('rules');
      if (!isSeq(rules)) continue;
      for (let j = rules.items.length - 1; j >= 0; j--)
        if (parseBlacklistRule(String(rules.items[j])).rule === rule) rules.delete(j);
      if (!rules.items.length) entries.delete(i);
    }
    if (save) commit();
  }
  function addGroup(name: string) {
    if (weightedIssue(name)) return;
    const target = groups.find((g) => g.name === name);
    if (!target) {
      void message.error('请选择节点组');
      return;
    }
    doc.setIn(['proxy-groups', target.index, 'type'], 'load-balance');
    doc.setIn(['proxy-groups', target.index, 'strategy'], 'weighted-sticky');
    commit();
    select(target.name);
    setAdding(false);
    setNewGroup(undefined);
  }
  function removeGroup(name: string) {
    const target = groups.find((g) => g.name === name);
    if (!target) return;
    doc.setIn(['proxy-groups', target.index, 'type'], 'select');
    doc.deleteIn(['proxy-groups', target.index, 'strategy']);
    doc.deleteIn(['proxy-groups', target.index, 'weighted-sticky']);
    if (isSeq(entries))
      for (let i = entries.items.length - 1; i >= 0; i--) {
        const entry = entries.items[i];
        if (isMap(entry) && entry.get('group') === name) entries.delete(i);
      }
    commit();
    select(undefined);
    setDraft(undefined);
  }
  function addRule() {
    if (!group || !draft) return;
    const value = draft.value.trim();
    if (!value || (!draft.kind.endsWith('-REGEX') && value.includes(',')) || /[\r\n]/.test(value)) {
      void message.error('请选择节点并填写有效的规则内容');
      return;
    }
    if (draft.kind === 'RULE-SET' && !providerNames.includes(value)) {
      const rule = rules.find((r) => r.name === value && r.blacklist);
      if (!rule || doc.hasIn(['rule-providers', value])) {
        void message.error('规则集不存在或现有同名规则集不支持黑名单');
        return;
      }
      const url = new URL(
        (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '') +
          '/rulesets/' +
          rule.code +
          '/' +
          encodeURIComponent(rule.filename),
        location.origin,
      ).href;
      doc.setIn(
        ['rule-providers', value],
        doc.createNode({
          type: 'http',
          behavior: 'classical',
          format: 'yaml',
          url,
          path: `./rules/users/${rule.code}.yaml`,
          interval: 86400,
        }),
      );
    }
    if (!doc.has('site-blacklist')) doc.set('site-blacklist', doc.createNode([]));
    const list = doc.get('site-blacklist');
    if (!isSeq(list)) {
      void message.error('site-blacklist 必须是列表');
      return;
    }
    if (draft.original) deleteRule(group.name, draft.original, false);
    const line = formatBlacklistRule(`${draft.kind},${value}`, draft.proxies);
    const entry = list.items.find((item) => isMap(item) && item.get('group') === group.name);
    if (isMap(entry)) entry.addIn(['rules'], line);
    else list.add(doc.createNode({ group: group.name, rules: [line] }));
    const problems = loadBalanceErrors(doc.toString());
    if (problems.length) {
      void message.error(problems[0]);
      return;
    }
    commit();
    setDraft(undefined);
  }
  return (
    <div className="template-entry-list">
      {!!errors.length && (
        <Alert
          type="error"
          showIcon
          title={errors.map((e) => (
            <div key={e}>{e}</div>
          ))}
        />
      )}
      {groups
        .filter(
          (g) =>
            g.type === 'load-balance' &&
            (editing ? selected === g.name : !visibleGroups || visibleGroups.includes(g.name)),
        )
        .map((g) => (
          <div
            key={g.name}
            className={[
              editing ? '' : 'template-route-card template-balance-card',
              entryClassName?.(g.name),
            ]
              .filter(Boolean)
              .join(' ')}
          >
            {!editing && (
              <div className="template-balance-heading">
                <div>
                  <div className="template-entry-title">{g.name}</div>
                  {selected !== g.name && (
                    <Space style={{ marginTop: 8 }}>
                      <Tag color="purple">黑名单 {ruleRows(g.name).length} 条</Tag>
                      <Tag color="blue">{g.strategy || 'consistent-hashing'}</Tag>
                    </Space>
                  )}
                </div>
                <Space size={4}>
                  {readonly && (
                    <Button
                      type="text"
                      aria-label={'查看 ' + g.name}
                      title={expanded.has(g.name) ? '收起黑名单规则' : '查看黑名单规则'}
                      aria-expanded={expanded.has(g.name)}
                      aria-controls={`${uid}-blacklist-${g.index}`}
                      className={expanded.has(g.name) ? 'template-edit-active' : undefined}
                      icon={<EyeOutlined aria-hidden />}
                      onClick={() =>
                        setExpanded((current) => {
                          const next = new Set(current);
                          if (next.has(g.name)) next.delete(g.name);
                          else next.add(g.name);
                          return next;
                        })
                      }
                    />
                  )}
                  {!readonly && (
                    <Space size={4}>
                      <Button
                        type="text"
                        aria-label={'设置 ' + g.name}
                        className={selected === g.name ? 'template-edit-active' : undefined}
                        aria-pressed={selected === g.name}
                        icon={<EditOutlined aria-hidden />}
                        onClick={() => {
                          onEdit?.(g.name);
                        }}
                      />
                      <Button
                        type="text"
                        danger
                        aria-label={'移除负载均衡 ' + g.name}
                        icon={<DeleteOutlined aria-hidden />}
                        onClick={() => removeGroup(g.name)}
                      />
                    </Space>
                  )}
                  {entryActions?.(g.name)}
                </Space>
              </div>
            )}
            {(editing || (readonly && expanded.has(g.name))) && (
              <div className="template-balance-rules" id={`${uid}-blacklist-${g.index}`}>
                {!readonly && (
                  <Space orientation="vertical" style={{ width: '100%' }}>
                    <label htmlFor={uid + '-group'}>节点组</label>
                    <Select
                      id={uid + '-group'}
                      aria-label="更换负载均衡节点组"
                      style={{ width: '100%' }}
                      value={g.name}
                      options={groups
                        .filter((item) => item.name === g.name || item.type !== 'load-balance')
                        .map((item) => groupOption(item.name, g.strategy === 'weighted-sticky'))}
                      optionRender={(option) => optionLabel(option.label, option.data.reason)}
                      onChange={(name) => changeGroup(g.name, name)}
                    />
                    <label htmlFor={uid + '-strategy'}>负载均衡类型</label>
                    <Select
                      id={uid + '-strategy'}
                      aria-label="负载均衡类型"
                      style={{ width: '100%' }}
                      value={g.strategy || 'consistent-hashing'}
                      options={[
                        'consistent-hashing',
                        'round-robin',
                        'sticky-sessions',
                        'weighted-sticky',
                      ].map((value) => ({
                        value,
                        label: value,
                        disabled: value === 'weighted-sticky' && !!weightedIssue(g.name),
                        reason: value === 'weighted-sticky' ? weightedIssue(g.name) : '',
                      }))}
                      optionRender={(option) => optionLabel(option.label, option.data.reason)}
                      onChange={(value) => {
                        if (value === 'weighted-sticky' && weightedIssue(g.name)) return;
                        doc.setIn(['proxy-groups', g.index, 'strategy'], value);
                        if (value !== 'weighted-sticky')
                          doc.deleteIn(['proxy-groups', g.index, 'weighted-sticky']);
                        commit();
                      }}
                    />
                  </Space>
                )}

                {g.strategy !== 'weighted-sticky' && (
                  <Alert
                    type="warning"
                    title="黑名单需要 weighted-sticky 策略"
                    action={
                      !readonly && (
                        <Tooltip title={weightedIssue(g.name)}>
                          <span>
                            <Button
                              disabled={!!weightedIssue(g.name)}
                              onClick={() => {
                                doc.setIn(['proxy-groups', g.index, 'strategy'], 'weighted-sticky');
                                commit();
                              }}
                            >
                              使用加权负载均衡
                            </Button>
                          </span>
                        </Tooltip>
                      )
                    }
                  />
                )}
                <strong>黑名单规则</strong>
                {ruleRows(g.name).map((e) => (
                  <div className="template-route-card template-blacklist-row" key={e.rule}>
                    <div className="template-blacklist-fields">
                      <div className="template-blacklist-field">
                        <span className="template-blacklist-label">路由规则</span>
                        <div className="template-blacklist-value">
                          <Tag color="blue">{e.rule.slice(0, e.rule.indexOf(','))}</Tag>
                          <span>{e.rule.slice(e.rule.indexOf(',') + 1)}</span>
                        </div>
                      </div>
                      <div className="template-blacklist-field">
                        <span className="template-blacklist-label">匹配节点</span>
                        <div className="template-blacklist-value">
                          {e.proxies.map((proxy) => (
                            <Tag color="purple" key={proxy}>
                              {proxy || '待发布时配置'}
                            </Tag>
                          ))}
                        </div>
                      </div>
                    </div>
                    {!readonly && (
                      <Space size={0}>
                        <Button
                          type="text"
                          aria-label="编辑黑名单"
                          icon={<EditOutlined aria-hidden />}
                          onClick={() =>
                            setDraft({
                              proxies: e.proxies.filter(Boolean),
                              kind: e.rule.slice(0, e.rule.indexOf(',')),
                              value: e.rule.slice(e.rule.indexOf(',') + 1),
                              original: e.rule,
                            })
                          }
                        />
                        <Button
                          type="text"
                          danger
                          aria-label="删除黑名单"
                          icon={<DeleteOutlined aria-hidden />}
                          onClick={() => deleteRule(g.name, e.rule)}
                        />
                      </Space>
                    )}
                  </div>
                ))}
                {selected === g.name && draft && !readonly && (
                  <Space
                    className="template-blacklist-form"
                    orientation="vertical"
                    style={{ width: '100%' }}
                  >
                    <Select
                      aria-label="黑名单避开节点"
                      placeholder="选择避开的代理节点（模板可留空）"
                      style={{ width: '100%' }}
                      mode="multiple"
                      value={draft.proxies}
                      options={(availableNodes?.[g.name] ?? g.effective).map((value) => ({
                        value,
                      }))}
                      onChange={(proxies: string[]) => {
                        setDraft({ ...draft, proxies });
                      }}
                    />
                    <Select
                      aria-label="黑名单规则类型"
                      style={{ width: '100%' }}
                      value={draft.kind}
                      options={['RULE-SET', ...blacklistRuleKinds].map((value) => ({ value }))}
                      onChange={(kind) => setDraft({ ...draft, kind, value: '' })}
                    />
                    {draft.kind === 'RULE-SET' ? (
                      <Select
                        aria-label="黑名单规则集"
                        showSearch
                        style={{ width: '100%' }}
                        placeholder="选择规则集"
                        value={draft.value || undefined}
                        options={names.map((value) => ({ value }))}
                        onChange={(value) => setDraft({ ...draft, value })}
                      />
                    ) : (
                      <Input
                        aria-label="黑名单规则内容"
                        placeholder={
                          draft.kind === 'DOMAIN-REGEX' ? '域名正则表达式' : 'example.com'
                        }
                        value={draft.value}
                        onChange={(e) => setDraft({ ...draft, value: e.target.value })}
                      />
                    )}
                    <Space>
                      <Button onClick={() => setDraft(undefined)}>取消</Button>
                      <Button type="primary" onClick={addRule}>
                        确认
                      </Button>
                    </Space>
                  </Space>
                )}
                {!readonly && !draft && (
                  <Button
                    block
                    icon={<PlusOutlined aria-hidden />}
                    disabled={g.strategy !== 'weighted-sticky'}
                    onClick={() => {
                      select(g.name);
                      setDraft({ proxies: [], kind: 'RULE-SET', value: '' });
                    }}
                  >
                    添加黑名单规则
                  </Button>
                )}
              </div>
            )}
          </div>
        ))}
      {adding && !readonly && (
        <Space orientation="vertical" style={{ width: '100%' }}>
          <label htmlFor={uid + '-group'}>节点组</label>
          <Select
            id={uid + '-group'}
            aria-label="负载均衡节点组"
            placeholder="选择节点组"
            style={{ width: '100%' }}
            value={newGroup}
            options={groups
              .filter((g) => g.type !== 'load-balance')
              .map((g) => groupOption(g.name, true))}
            optionRender={(option) => optionLabel(option.label, option.data.reason)}
            onChange={(name) => {
              setNewGroup(name);
              addGroup(name);
            }}
          />
          <label htmlFor={uid + '-strategy'}>负载均衡类型</label>
          <Select
            id={uid + '-strategy'}
            aria-label="负载均衡类型"
            style={{ width: '100%' }}
            value="weighted-sticky"
            disabled
            options={[{ value: 'weighted-sticky' }]}
          />
          <strong>黑名单规则</strong>
          <Button block disabled icon={<PlusOutlined aria-hidden />}>
            添加黑名单规则
          </Button>
        </Space>
      )}
      {!readonly && !editing && (
        <Button
          className="template-entry-add"
          icon={<PlusOutlined aria-hidden />}
          disabled={!!doc.errors.length}
          onClick={() => onEdit?.()}
        >
          添加负载均衡
        </Button>
      )}
    </div>
  );
}
