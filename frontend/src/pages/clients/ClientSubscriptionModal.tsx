import { useState } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import {
  useClashSubscription,
  useSaveClashSubscription,
} from '@/api/queries/useClashSubscriptions';
import { validateSubscription } from '@/lib/clash/subscription/api';
import type { Template, RuleSet, SubscriptionProxy } from '@/schemas/clash/subscription';
import {
  Alert,
  message,
  Button,
  Segmented,
  Modal,
  Select,
  Space,
  Spin,
  Tabs,
  Tag,
  Typography,
  Form,
} from 'antd';
import { CheckOutlined, CopyOutlined, StopOutlined, UndoOutlined } from '@ant-design/icons';
import { stringify } from 'yaml';
import type { ClientRecord } from '@/hooks/useClients';
import TemplateInsert from '@/components/clash/subscription/TemplateInsert';
import NodeGroups from '@/components/clash/subscription/NodeGroups';
import TemplateWorkspace from '@/components/clash/subscription/TemplateWorkspace';
import LoadBalance from '@/components/clash/subscription/LoadBalance';
import { deleteSubscriptionGroup } from '@/lib/clash/subscription/subscriptionDeletion';
import { validateTemplate } from '@/lib/clash/subscription/templateValidation';
import {
  composeSubscription,
  disabledField,
  documentObject,
  entries,
  setEntries,
  ruleIdentity,
  subscriptionFromYAML,
  type Section,
} from '@/lib/clash/subscription/subscriptionModel';
import { emptySubscription, type Subscription } from '@/schemas/clash/subscription';
import '@/components/clash/subscription/Templates.css';
import './ClientSubscriptionModal.css';

interface Props {
  client: ClientRecord;
  subscriptionURL?: string;
  onClose: () => void;
}
export default function ClientSubscriptionModal(props: Props) {
  const query = useClashSubscription(props.client.id);
  return (
    <Modal
      open
      title={`Clash 订阅 · ${props.client.email}`}
      width={1240}
      centered
      footer={null}
      onCancel={props.onClose}
      destroyOnHidden
    >
      {query.isPending && <Spin />}
      {query.error && <Alert type="error" title={query.error.message} />}
      {query.data && <SubscriptionEditor {...props} data={query.data} />}
    </Modal>
  );
}

function SubscriptionEditor({
  client,
  subscriptionURL,
  onClose,
  data,
}: Props & {
  data: {
    proxies: SubscriptionProxy[];
    binding: Subscription;
    templates: Template[];
    inbounds: { inboundKey: string; name: string; hostOverride: boolean }[];
    rules: RuleSet[];
  };
}) {
  const form = useForm<Subscription>({ defaultValues: data.binding });
  const [modal, modalContextHolder] = Modal.useModal();
  const draft = useWatch({ control: form.control }) as Subscription;
  const [saving, setSaving] = useState(false);
  const [chainValid, setChainValid] = useState(true);
  const [activeTab, setActiveTab] = useState('subscription');
  const [source, setSource] = useState<'template' | 'custom'>('template');
  const [yamlDraft, setYamlDraft] = useState<string>();
  const [yamlError, setYamlError] = useState('');
  const changeBinding: typeof form.setValue = (...args) => {
    setYamlDraft(undefined);
    setYamlError('');
    form.setValue(...args);
  };
  const [error, setError] = useState('');
  const saveMutation = useSaveClashSubscription(client.id);
  const template = data.templates.find((item) => item.id === draft.templateId);
  const overlay = documentObject(draft.content);
  const base = documentObject(template?.content ?? '{}');
  let composed = '{}',
    compositionError = '';
  try {
    composed = composeSubscription(template?.content ?? '{}', draft);
  } catch (e) {
    compositionError = String(e);
  }
  const combined = documentObject(composed);
  const groupRows = entries(combined, 'proxy-groups').map(([name, value]) => ({
    name,
    type: String((value as Record<string, unknown>).type),
  }));
  const injectedMembers = Object.fromEntries(
    groupRows.map((group) => {
      const injection = draft.groups.find((item) => item.group === group.name);
      const original = entries(combined, 'proxy-groups').find(
        ([name]) => name === group.name,
      )?.[1] as Record<string, unknown>;
      const members = Array.isArray(original.proxies)
        ? original.proxies.filter((v): v is string => typeof v === 'string')
        : [];
      return [
        group.name,
        [
          ...new Set([
            ...members,
            ...data.proxies
              .filter((p) => injection?.allNodes || injection?.nodes.includes(p.inboundKey))
              .map((p) => p.name),
          ]),
        ],
      ];
    }),
  );
  const selectedProxies = data.proxies.filter(
    (p) =>
      draft.groups.some((g) => g.allNodes || g.nodes.includes(p.inboundKey)) ||
      Object.entries(draft.dialers).some(
        ([source, target]) => !!target && (source === p.name || target === p.name),
      ),
  );
  const generatedProxies = selectedProxies.map((p) => ({
    ...p.config,
    ...(draft.dialers[p.name] ? { 'dialer-proxy': draft.dialers[p.name] } : {}),
  }));
  const templateProxies = Array.isArray(combined.proxies) ? combined.proxies : [];
  const previewProxies = [...templateProxies, ...generatedProxies].map((value) => {
    const proxy = { ...(value as Record<string, unknown>) };
    const name = String(proxy.name);
    if (Object.hasOwn(draft.dialers, name)) {
      if (draft.dialers[name]) proxy['dialer-proxy'] = draft.dialers[name];
      else delete proxy['dialer-proxy'];
    }
    return proxy;
  });
  const previewContent = stringify({
    ...combined,
    proxies: previewProxies,
    'proxy-groups': entries(combined, 'proxy-groups').map(([name, value]) => ({
      ...(value as Record<string, unknown>),
      proxies: injectedMembers[name],
    })),
  });
  const chainContent = stringify({
    ...combined,
    proxies: previewProxies,
    'proxy-groups': entries(combined, 'proxy-groups').map(([name, value]) => ({
      ...(value as Record<string, unknown>),
      proxies: injectedMembers[name],
    })),
  });
  const customGroups = entries(overlay, 'proxy-groups').map(([name, value]) => ({
    name,
    type: String((value as Record<string, unknown>).type),
  }));

  function changeSection(section: Section, content: string) {
    try {
      const updated = documentObject(content);
      const next = setEntries(overlay, section, entries(updated, section));
      const remaining = new Set(entries(updated, section).map(([id]) => id));
      const restored = new Set(
        entries(overlay, section)
          .filter(([id]) => !remaining.has(id))
          .map(([id]) => id),
      );
      const field = disabledField[section];
      changeBinding(
        field,
        draft[field].filter((id) => !restored.has(id)),
        { shouldDirty: true },
      );
      changeBinding('content', stringify(next), { shouldDirty: true });
      setError('');
    } catch (e) {
      setError(String(e));
    }
  }
  function removeGroup(name: string) {
    try {
      const current = form.getValues();
      const result = deleteSubscriptionGroup(template?.content ?? '{}', current, name);
      const apply = () => {
        if (JSON.stringify(form.getValues()) !== JSON.stringify(current))
          throw new Error('配置已变更，请重新执行删除');
        setYamlDraft(undefined);
        setYamlError('');
        form.reset(result.binding);
      };
      if (result.restored || !result.references.length) {
        apply();
        return;
      }
      modal.confirm({
        title: `删除代理组「${name}」及其引用？`,
        content: (
          <div>
            <p>以下引用将一并删除，不会改为直连。其他代理组若因此为空，需要重新配置成员。</p>
            <ul
              style={{
                maxHeight: '40vh',
                overflowY: 'auto',
                overflowWrap: 'anywhere',
              }}
            >
              {result.references.map((reference, index) => (
                <li key={index}>{reference}</li>
              ))}
            </ul>
          </div>
        ),
        okText: '删除代理组及引用',
        cancelText: '取消',
        okButtonProps: { danger: true },
        onOk: apply,
      });
    } catch (e) {
      void message.error(String(e));
    }
  }
  function inherit(section: Section, balance = false) {
    const rows = entries(base, section).filter(
      ([, value]) => !balance || (value as Record<string, unknown>).type === 'load-balance',
    );
    const custom = new Set(entries(overlay, section).map(([id]) => id));
    const field = disabledField[section];
    const identity = (name: string) => (section === 'rules' ? ruleIdentity(name) : name);
    const entryClassName = (name: string) =>
      'subscription-inherited-item ' +
      (custom.has(identity(name))
        ? 'subscription-inherited-overridden'
        : draft[field].includes(identity(name))
          ? 'subscription-inherited-disabled'
          : '');
    const toggleBalance = (name: string) => {
      const current = entries(combined, 'proxy-groups').find(([id]) => id === name)?.[1] as
        | Record<string, unknown>
        | undefined;
      const inherited = rows.find(([id]) => id === name)?.[1] as Record<string, unknown>;
      const enabled = current?.type === 'load-balance';
      const group = { ...(current ?? inherited) };
      const blacklists = Array.isArray(combined['site-blacklist'])
        ? (combined['site-blacklist'] as Record<string, unknown>[])
        : [];
      let nextRules = blacklists.filter((entry) => entry.group !== name);
      if (enabled || !current) {
        group.type = 'select';
        delete group.strategy;
        delete group['weighted-sticky'];
      } else {
        group.type = 'load-balance';
        for (const field of ['strategy', 'weighted-sticky'])
          if (field in inherited) group[field] = inherited[field];
        const originalRules = Array.isArray(base['site-blacklist'])
          ? (base['site-blacklist'] as Record<string, unknown>[])
          : [];
        nextRules = [...nextRules, ...originalRules.filter((entry) => entry.group === name)];
      }
      const next = setEntries(overlay, 'proxy-groups', [
        ...entries(overlay, 'proxy-groups').filter(([id]) => id !== name),
        [name, group],
      ]);
      next['site-blacklist'] = nextRules;
      changeBinding('content', stringify(next), { shouldDirty: true });
      changeBinding(
        'disabledGroups',
        draft.disabledGroups.filter((id) => id !== name),
        { shouldDirty: true },
      );
      setError('');
    };
    const entryActions = (name: string) => {
      const id = identity(name),
        disabled = draft[field].includes(id),
        overridden = custom.has(id);
      return (
        <Space size={0} className="subscription-entry-actions">
          {balance ? (
            <Button
              size="small"
              type="text"
              aria-label={
                (
                  entries(combined, 'proxy-groups').find(([key]) => key === id)?.[1] as
                    | Record<string, unknown>
                    | undefined
                )?.type === 'load-balance'
                  ? '关闭负载均衡'
                  : '开启负载均衡'
              }
              title="切换负载均衡，保留代理组"
              icon={<StopOutlined aria-hidden />}
              onClick={() => toggleBalance(id)}
            />
          ) : (
            <Button
              size="small"
              type="text"
              danger={!disabled && !overridden}
              disabled={overridden}
              title={overridden ? '已覆盖' : disabled ? '恢复继承' : '禁用'}
              aria-label={overridden ? '已覆盖' : disabled ? '恢复继承' : '禁用'}
              icon={
                overridden ? (
                  <CheckOutlined aria-hidden />
                ) : disabled ? (
                  <UndoOutlined aria-hidden />
                ) : (
                  <StopOutlined aria-hidden />
                )
              }
              onClick={() =>
                changeBinding(
                  field,
                  disabled ? draft[field].filter((key) => key !== id) : [...draft[field], id],
                  { shouldDirty: true },
                )
              }
            />
          )}
          <Button
            size="small"
            type="text"
            icon={<CopyOutlined aria-hidden />}
            title="复制到自定义编辑"
            aria-label="覆盖"
            disabled={overridden}
            onClick={() => {
              setSource('custom');
              const value = rows.find(([key]) => key === id)?.[1];
              changeBinding(
                'content',
                stringify(
                  setEntries(overlay, section, [...entries(overlay, section), [id, value]]),
                ),
                { shouldDirty: true },
              );
            }}
          />
        </Space>
      );
    };
    const inheritedContent = stringify(setEntries(base, section, rows));
    const common = {
      content: inheritedContent,
      readonly: true,
      entryActions,
      entryClassName,
    };
    return section === 'proxy-groups' ? (
      balance ? (
        <LoadBalance {...common} rules={data.rules} onChange={() => {}} />
      ) : (
        <NodeGroups
          {...common}
          groups={rows.map(([name, value]) => ({
            name,
            type: String((value as Record<string, unknown>).type),
          }))}
          bindings={[]}
          invalid={false}
          change={() => {}}
        />
      )
    ) : (
      <TemplateInsert
        {...common}
        tab={section === 'rules' ? 'routes' : 'providers'}
        groups={groupRows.map((g) => g.name)}
        rules={data.rules}
        onChange={() => {}}
      />
    );
  }

  function routeEditor(section: 'rules' | 'rule-providers') {
    if (source === 'template') return inherit(section);
    const context = {
      ...combined,
      [section]: overlay[section] ?? (section === 'rules' ? [] : {}),
    };
    return (
      <>
        <TemplateInsert
          content={stringify(context)}
          onChange={(value) => changeSection(section, value)}
          tab={section === 'rules' ? 'routes' : 'providers'}
          groups={groupRows.map((g) => g.name)}
          rules={data.rules}
          readonly={false}
        />
      </>
    );
  }
  async function check() {
    if (!draft.templateId) {
      void message.success('使用官方默认订阅');
      return;
    }
    try {
      if (!chainValid) throw Error('请先修复链式代理配置');
      await validateSubscription(client.id, draft);
      setError('');
      void message.success('订阅配置校验通过');
    } catch (e) {
      setError(String(e));
    }
  }
  async function save() {
    setSaving(true);
    setError('');
    try {
      if (yamlError) throw new Error(yamlError);
      if (!chainValid) throw new Error('请先修复链式代理配置');
      if (draft.templateId) {
        if (!template) throw new Error('请选择有效模板');
        if (compositionError) throw new Error(compositionError);
        const report = validateTemplate(composed);
        if (report.errors.length) throw new Error(report.errors.join('\n'));
        const missing = report.pending.filter(
          (name) => !draft.groups.some((g) => g.group === name && (g.allNodes || g.nodes.length)),
        );
        if (missing.length) throw new Error('请为代理组配置注入：' + missing.join('、'));
      }
      await saveMutation.mutateAsync(draft);
      void message.success('订阅配置已保存');
      onClose();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }
  const formatTime = (value: string | number) =>
    new Date(value).toLocaleString('zh-CN', {
      year: 'numeric',
      month: '2-digit',
      day: '2-digit',
      hour: '2-digit',
      minute: '2-digit',
      hour12: false,
    });
  const settings = (
    <Form layout="vertical" className="template-entry-form">
      <Form.Item label="订阅模板">
        <Select
          aria-label="选择订阅模板"
          value={draft.templateId}
          options={[
            { value: 0, label: '官方默认订阅' },
            ...data.templates.map((item) => ({
              value: item.id,
              label: item.name,
            })),
          ]}
          onChange={(templateId) => {
            setYamlDraft(undefined);
            setYamlError('');
            setSource('template');
            form.reset({ ...emptySubscription, templateId });
          }}
        />
      </Form.Item>
      <Form.Item label="订阅地址">
        {subscriptionURL ? (
          <Typography.Paragraph
            className="subscription-address"
            copyable={{ text: subscriptionURL }}
          >
            {subscriptionURL}
          </Typography.Paragraph>
        ) : (
          <Typography.Text type="secondary">尚未启用 Clash 订阅地址</Typography.Text>
        )}
      </Form.Item>
      <Form.Item label="更新时间">
        <Tag className="subscription-time-tag">
          {data.binding.updatedAt ? formatTime(data.binding.updatedAt) : '尚未保存'}
        </Tag>
      </Form.Item>
      <Form.Item
        label={
          <span>
            客户端
            <br />
            同步时间
          </span>
        }
      >
        <Tag className="subscription-time-tag">
          {data.binding.lastSyncAt ? formatTime(data.binding.lastSyncAt) : '尚未同步'}
        </Tag>
      </Form.Item>
    </Form>
  );
  return (
    <>
      {modalContextHolder}
      <TemplateWorkspace
        content={yamlDraft ?? previewContent}
        knownProxies={selectedProxies.map((p) => p.name)}
        onChange={(content) => {
          setYamlDraft(content);
          try {
            const edited = documentObject(content);
            if (JSON.stringify(edited.proxies) !== JSON.stringify(previewProxies))
              throw new Error('代理协议配置由 3x-ui 官方订阅生成，请在入站管理中修改');
            if ('proxies' in combined) edited.proxies = combined.proxies;
            else delete edited.proxies;
            for (const [name, value] of entries(edited, 'proxy-groups')) {
              const group = value as Record<string, unknown>;
              const original = entries(combined, 'proxy-groups').find(
                ([id]) => id === name,
              )?.[1] as Record<string, unknown> | undefined;
              const explicit = Array.isArray(original?.proxies) ? original.proxies : [];
              const generated = (injectedMembers[name] ?? []).filter(
                (member) => !explicit.includes(member),
              );
              if (
                generated.length &&
                (!Array.isArray(group.proxies) ||
                  generated.some((member) => !(group.proxies as unknown[]).includes(member)))
              )
                throw new Error('已分配代理由代理组选择生成，请在右侧代理组编辑中调整');
              if (Array.isArray(group.proxies))
                group.proxies = group.proxies.filter(
                  (member) => !generated.includes(String(member)),
                );
            }
            const next = subscriptionFromYAML(template?.content ?? '{}', stringify(edited), draft);

            form.reset(next);
            setYamlError('');
          } catch (e) {
            setYamlError(String(e));
          }
        }}
        readonly={!template}
        error={yamlError || compositionError}
        groupCount={groupRows.length}
      >
        <Tabs
          activeKey={activeTab}
          onChange={setActiveTab}
          items={[
            { key: 'subscription', label: '订阅设置', children: settings },
            {
              key: 'groups',
              label: '代理组',
              disabled: !template,
              children:
                source === 'template' ? (
                  inherit('proxy-groups')
                ) : (
                  <>
                    <NodeGroups
                      content={draft.content}
                      referenceContent={composed}
                      entryClassName={() => 'subscription-node-card'}
                      onRemove={removeGroup}
                      groups={customGroups}
                      bindings={draft.groups}
                      inboundOptions={data.inbounds}
                      readonly={false}
                      invalid={!!compositionError}
                      change={(content, bindings) => {
                        changeSection('proxy-groups', content);
                        changeBinding('groups', bindings, {
                          shouldDirty: true,
                        });
                      }}
                    />
                  </>
                ),
            },
            {
              key: 'providers',
              label: '规则集',
              disabled: !template,
              children: routeEditor('rule-providers'),
            },
            {
              key: 'routes',
              label: '路由',
              disabled: !template,
              children: routeEditor('rules'),
            },
            {
              key: 'load-balance',
              label: '负载均衡',
              disabled: !template,
              children:
                source === 'template' ? (
                  inherit('proxy-groups', true)
                ) : (
                  <>
                    <LoadBalance
                      visibleGroups={customGroups.map((g) => g.name)}
                      availableNodes={injectedMembers}
                      content={composed}
                      readonly={false}
                      rules={data.rules}
                      onChange={(content) => {
                        const next = subscriptionFromYAML(
                          template?.content ?? '{}',
                          content,
                          draft,
                        );
                        changeBinding('content', next.content, {
                          shouldDirty: true,
                        });
                      }}
                    />
                  </>
                ),
            },
            {
              key: 'dialer',
              label: '链式代理',
              disabled: !template,
              children: (
                <TemplateInsert
                  content={chainContent}
                  tab="dialer"
                  groups={groupRows.map((g) => g.name)}
                  rules={data.rules}
                  readonly={false}
                  onValidityChange={setChainValid}
                  onChange={(content) => {
                    const updated = documentObject(content);
                    const proxies = updated.proxies as Record<string, unknown>[];
                    changeBinding(
                      'dialers',
                      Object.fromEntries(
                        proxies
                          .filter(
                            (p) =>
                              typeof p['dialer-proxy'] === 'string' ||
                              Object.hasOwn(draft.dialers, String(p.name)) ||
                              previewProxies.some(
                                (original) =>
                                  original.name === p.name &&
                                  typeof original['dialer-proxy'] === 'string',
                              ),
                          )
                          .map((p) => [String(p.name), String(p['dialer-proxy'] ?? '')]),
                      ),
                      { shouldDirty: true },
                    );
                  }}
                />
              ),
            },
          ]}
        />
        {template && ['groups', 'providers', 'routes', 'load-balance'].includes(activeTab) && (
          <div className="subscription-source-toggle">
            <Segmented<'template' | 'custom'>
              size="small"
              aria-label="配置来源"
              value={source}
              onChange={setSource}
              options={[
                { label: '模板', value: 'template' },
                { label: '自定义', value: 'custom' },
              ]}
            />
          </div>
        )}
      </TemplateWorkspace>
      {(error || yamlError || compositionError) && (
        <Alert
          style={{ marginTop: 12 }}
          type="error"
          title={error || yamlError || compositionError}
        />
      )}
      <div className="subscription-footer">
        <Space>
          <Button onClick={onClose}>取消</Button>
          <Button onClick={() => void check()}>校验配置</Button>
          <Button
            type="primary"
            disabled={!chainValid}
            loading={saving}
            onClick={() => void save()}
          >
            保存订阅
          </Button>
        </Space>
      </div>
    </>
  );
}
