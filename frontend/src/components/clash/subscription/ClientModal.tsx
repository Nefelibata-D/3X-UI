import { useImperativeHandle, useRef, useState } from 'react';
import {
  Alert,
  Button,
  Checkbox,
  Select,
  Space,
  Tag,
  Typography,
  Form,
  ConfigProvider,
  Dropdown,
  Modal,
  Spin,
  Tabs,
  message,
  theme,
} from 'antd';
import { EditOutlined, PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useClashSubscription } from '@/api/queries/clash/subscription';
import { changeProfile } from '@/lib/clash/subscription/api';
import { RandomUtil } from '@/utils';
import { emptySubscription, type Profile, type Subscription } from '@/schemas/clash/subscription';
import type { ClientRecord } from '@/hooks/useClients';

import { useForm, useWatch } from 'react-hook-form';

import { injectPublication, restorePublicationFields } from '@/lib/clash/subscription/publication';
import { stringify } from 'yaml';

import TemplateInsert from '@/components/clash/subscription/TemplateInsert';
import SubscriptionEntries from '@/components/clash/subscription/SubscriptionEntries';
import TemplateWorkspace from './TemplateWorkspace';
import { deleteSubscriptionGroup } from '@/lib/clash/subscription/deletion';
import { validateTemplate } from '@/lib/clash/subscription/template/validation';
import {
  composeSubscription,
  documentObject,
  entries,
  subscriptionFromYAML,
  type Section,
} from '@/lib/clash/subscription/model';
import '@/components/clash/subscription/Templates.css';
import '@/components/clash/subscription/ClientModal.css';

interface Props {
  client: ClientRecord;
  subscriptionURL?: string;
  onClose: () => void;
}
type Data = NonNullable<ReturnType<typeof useClashSubscription>['data']>;
type DraftProfile = Profile & { isNew?: boolean; pendingToken?: string };
export default function ClientSubscriptionModal(props: Props) {
  const query = useClashSubscription(props.client.id);
  return (
    <ConfigProvider theme={{ inherit: false, algorithm: theme.defaultAlgorithm }}>
      <Modal
        open
        centered
        aria-label="Clash 订阅"
        width={1240}
        footer={null}
        onCancel={props.onClose}
      >
        {query.isPending && <Spin />}
        {query.error && <Alert type="error" title={query.error.message} />}
        {query.data && props.client.id && (
          <ProfileTabs clientId={props.client.id} {...props} data={query.data} />
        )}
      </Modal>
    </ConfigProvider>
  );
}
function ProfileTabs({
  client,
  clientId,
  subscriptionURL,
  onClose,
  data,
}: Props & { data: Data; clientId: number }) {
  const [profiles, setProfiles] = useState<DraftProfile[]>(data.profiles);
  const selectionKey = 'clash-subscription-profile:' + clientId;
  const [active, setActive] = useState(() => {
    const previous = sessionStorage.getItem(selectionKey);
    return profiles.find((profile) => profile.key === previous)?.key ?? profiles[0].key;
  });
  const [visited, setVisited] = useState(() => new Set([active]));
  function select(key: string) {
    sessionStorage.setItem(selectionKey, key);
    setActive(key);
    setVisited((prev) => new Set(prev).add(key));
  }
  const [editing, setEditing] = useState('');
  const [busy, setBusy] = useState(false);
  const cache = useQueryClient();
  const reader = useRef<{ read: () => Subscription | undefined }>(null);
  const draft = profiles.find((p) => p.key === active)!;
  const latest = profiles
    .filter((p) => !p.isNew)
    .reduce<DraftProfile | undefined>(
      (a, b) => (!a || a.updatedAt < b.updatedAt ? b : a),
      undefined,
    );
  function update(key: string, changes: Partial<DraftProfile>) {
    setProfiles((all) => all.map((p) => (p.key === key ? { ...p, ...changes } : p)));
  }
  function add(mode: string) {
    const source = mode === 'current' ? draft : latest;
    const binding = mode === 'current' ? reader.current?.read() : source;
    if (!source || !binding) {
      void message.error('请先修复当前配置');
      return;
    }
    const key = RandomUtil.randomUUID();
    setProfiles((all) => [
      ...all,
      {
        ...structuredClone(binding),
        key,
        name: source.name + ' 副本',
        token: RandomUtil.randomSeq(20),
        updatedAt: 0,
        lastSyncAt: 0,
        isNew: true,
      },
    ]);
    select(key);
  }
  const [removed, setRemoved] = useState<{ key: string; expected: number }[]>([]);
  function remove(key: string) {
    const profile = profiles.find((p) => p.key === key);
    if (!profile || profiles.length < 2 || busy) return;
    if (!profile.isNew) setRemoved((list) => [...list, { key, expected: profile.updatedAt }]);
    const rest = profiles.filter((p) => p.key !== key);
    setProfiles(rest);
    if (active === key) select(rest[0].key);
  }
  async function save(p: DraftProfile, binding: Subscription, rotate: boolean) {
    setBusy(true);
    try {
      const result = await changeProfile(
        clientId,
        { ...binding, key: p.key, name: p.name, token: rotate ? p.pendingToken : p.token },
        p.updatedAt,
        p.isNew,
        removed,
      );
      if (!result) throw Error('订阅保存失败');
      sessionStorage.setItem(selectionKey, p.key);
      setRemoved([]);
      update(p.key, { ...result, isNew: false, pendingToken: undefined });
      await cache.invalidateQueries({ queryKey: keys.clash.subscription(clientId) });
    } finally {
      setBusy(false);
    }
  }
  const plus = (
    <Dropdown
      trigger={['click']}
      disabled={busy}
      menu={{
        items: [
          { key: 'current', label: '复制当前订阅' },
          { key: 'latest', label: '复制最近保存的订阅', disabled: !latest },
        ],
        onClick: ({ key }) => add(key),
      }}
    >
      <Button aria-label="添加订阅" type="text" icon={<PlusOutlined />} />
    </Dropdown>
  );
  return (
    <>
      <Tabs
        className="profile-tabs"
        type="editable-card"
        activeKey={active}
        hideAdd
        onChange={select}
        tabBarExtraContent={plus}
        onEdit={(key, action) => {
          if (typeof key === 'string' && action === 'remove') void remove(key);
        }}
        items={profiles.map((p) => ({
          key: p.key,
          closable:
            !busy &&
            (p.isNew ? profiles.length > 1 : profiles.filter((item) => !item.isNew).length > 1),
          label: (
            <span className="profile-label">
              <span>
                {p.name} {active === p.key && <EditOutlined />}
              </span>
              {active === p.key && (
                <input
                  aria-label="订阅名称"
                  defaultValue={p.name}
                  className={editing === p.key ? 'editing' : ''}
                  onFocus={(e) => {
                    setEditing(p.key);
                    e.target.select();
                  }}
                  onClick={(e) => e.stopPropagation()}
                  onBlur={(e) => {
                    const name = e.target.value.trim() || p.name;
                    e.target.value = name;
                    update(p.key, { name });
                    setEditing('');
                  }}
                  onKeyDown={(e) => {
                    e.stopPropagation();
                    if (e.key === 'Escape') {
                      e.currentTarget.value = p.name;
                      e.currentTarget.blur();
                    }
                    if (e.key === 'Enter') e.currentTarget.blur();
                  }}
                />
              )}
            </span>
          ),
        }))}
      />
      {profiles
        .filter((p) => visited.has(p.key))
        .map((p) => {
          const token = p.pendingToken ?? p.token;
          const address =
            subscriptionURL &&
            token && new URL(token, new URL(subscriptionURL, window.location.href)).href;
          return (
            <div key={p.key} hidden={active !== p.key}>
              <SubscriptionEditor
                client={client}
                subscriptionURL={subscriptionURL}
                generalURL={address || undefined}
                pendingRotation={!!p.pendingToken}
                onRotate={() => update(p.key, { pendingToken: RandomUtil.randomSeq(20) })}
                onClose={onClose}
                draftRef={active === p.key ? reader : null}
                data={{ ...data, binding: p }}
                onSave={(binding, rotate) => save(p, binding, rotate)}
              />
            </div>
          );
        })}
    </>
  );
}

function SubscriptionEditor({
  generalURL,
  client,
  subscriptionURL,
  pendingRotation,
  onRotate,
  onClose,
  data,
  draftRef,
  onSave,
}: Props & {
  generalURL?: string;
  pendingRotation: boolean;
  onRotate: () => void;
  draftRef: React.Ref<{ read: () => Subscription | undefined }>;
  onSave: (draft: Subscription, rotate: boolean) => Promise<void>;
  data: Omit<Data, 'profiles'> & { binding: Subscription };
}) {
  const { token } = theme.useToken();
  const [modal, modalContextHolder] = Modal.useModal();
  const form = useForm<Subscription>({ defaultValues: data.binding });
  const draft = useWatch({ control: form.control }) as Subscription;
  const [saving, setSaving] = useState(false);
  const [chainValid, setChainValid] = useState(true);
  const [activeTab, setActiveTab] = useState('subscription');
  const [showTemplate, setShowTemplate] = useState(true);
  const [showCustom, setShowCustom] = useState(true);
  const [yamlDraft, setYamlDraft] = useState<string>();
  const [yamlError, setYamlError] = useState('');
  useImperativeHandle(draftRef, () => ({
    read: () => (yamlError ? undefined : form.getValues()),
  }));
  const changeBinding: typeof form.setValue = (...args) => {
    setYamlDraft(undefined);
    setYamlError('');
    form.setValue(...args);
  };
  const [error, setError] = useState('');

  const template = data.templates.find((item) => item.id === draft.templateId);
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
  const memberContent = stringify({
    ...combined,
    proxies: previewProxies,
    'proxy-groups': entries(combined, 'proxy-groups').map(([name, value]) => ({
      ...(value as Record<string, unknown>),
      proxies: injectedMembers[name],
    })),
  });
  const chainContent = memberContent;
  let previewContent = memberContent,
    publicationError = '';
  try {
    const published = injectPublication(
      documentObject(memberContent),
      data.publication,
      Object.fromEntries(data.proxies.map((proxy) => [proxy.name, proxy.nodeKey])),
    );
    const content = stringify(published);
    const report = validateTemplate(content, 'subscription');
    if (report.errors.length) throw Error(report.errors.join('\n'));
    previewContent = content;
  } catch (e) {
    publicationError = String(e);
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
  function entryEditor(section: Section, balance = false) {
    return (
      <SubscriptionEntries
        key={balance ? 'balance' : section}
        section={section}
        balance={balance}
        base={template?.content ?? '{}'}
        binding={draft}
        combined={combined}
        groups={groupRows}
        rules={data.rules}
        inbounds={data.inbounds}
        memberContent={memberContent}
        availableNodes={injectedMembers}
        managedProxyNames={data.proxies.map((p) => p.name)}
        onRemove={removeGroup}
        showTemplate={showTemplate}
        showCustom={showCustom}
        change={(next) => {
          setYamlDraft(undefined);
          setYamlError('');
          form.reset(next);
        }}
      />
    );
  }
  async function save() {
    setSaving(true);
    setError('');
    try {
      if (yamlError || publicationError) throw new Error(yamlError || publicationError);
      if (!chainValid) throw new Error('请先修复链式代理配置');
      if (draft.templateId) {
        if (!template) throw new Error('请选择有效模板');
        if (compositionError) throw new Error(compositionError);
      }

      let rotate = false;
      if (pendingRotation) {
        const choice = await new Promise<'keep' | 'rotate' | 'cancel'>((resolve) => {
          const dialog = modal.confirm({
            centered: true,
            title: '保存订阅',
            content: '轮转后旧链接将失效。你也可以保留原链接，仅保存配置。',
            okText: '保存并轮转',
            cancelText: '取消',
            onOk: () => {
              resolve('rotate');
            },
            onCancel: () => {
              resolve('cancel');
            },
            footer: (_, { CancelBtn, OkBtn }) => (
              <div className="rotation-actions" style={{ gap: token.marginSM }}>
                <CancelBtn />
                <Button
                  style={{ background: token.colorWarning, color: token.colorText }}
                  onClick={() => {
                    resolve('keep');
                    dialog.destroy();
                  }}
                >
                  保存但不轮转
                </Button>
                <OkBtn />
              </div>
            ),
          });
        });
        if (choice === 'cancel') return;
        rotate = choice === 'rotate';
      }
      await onSave(draft, rotate);
      void message.success('订阅配置已保存');
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
    <Form layout="horizontal" colon={false} className="template-entry-form subscription-settings">
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
            setShowTemplate(true);
            setShowCustom(true);
            form.reset({ ...emptySubscription, uaKeywords: draft.uaKeywords, templateId });
          }}
        />
      </Form.Item>
      <Form.Item label="客户端">
        <Typography.Text>{client.email}</Typography.Text>
      </Form.Item>
      <Form.Item label="独立订阅">
        {subscriptionURL ? <Typography.Text copyable>{subscriptionURL}</Typography.Text> : '尚未启用'}
      </Form.Item>
      <Form.Item label="通用订阅">
        {generalURL ? (
          <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
            <span
              aria-label="完整订阅地址"
              title={generalURL}
              style={{
                color: pendingRotation ? '#237804' : undefined,
                minWidth: 0,
                overflow: 'hidden',
                whiteSpace: 'nowrap',
                textOverflow: 'ellipsis',
              }}
            >
              {generalURL}
            </span>
            {pendingRotation && <Tag color="green">待保存</Tag>}
            <Typography.Text copyable={{ text: generalURL }} />
            <Button
              aria-label="轮转链接"
              title="轮转链接"
              size="small"
              type="link"
              icon={<ReloadOutlined />}
              onClick={onRotate}
            />
          </div>
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
      </Form.Item>{' '}
      <Form.Item label="UA 关键词">
        <Select
          mode="tags"
          aria-label="UA 关键词"
          value={draft.uaKeywords}
          placeholder="输入关键词后按 Enter"
          options={['FlClash/v{x.y.z}', 'clash-verge/v{x.y.z.a}', 'Shadowrocket'].map((value) => ({ value, label: value }))}
          onChange={(values: string[]) => {
            const unique = new Map<string, string>();
            for (const item of values) {
              const word = item.trim();
              if (word) unique.set(word.toLowerCase(), word);
            }
            form.setValue('uaKeywords', [...unique.values()], { shouldDirty: true });
          }}
        />
      </Form.Item>
    </Form>
  );
  return (
    <>
      {modalContextHolder}
      <TemplateWorkspace
        light
        title="编辑器"
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
              const published = entries(documentObject(previewContent), 'proxy-groups').find(
                ([id]) => id === name,
              )?.[1] as Record<string, unknown> | undefined;
              restorePublicationFields(group, original ?? {}, published ?? {});
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
        error={yamlError || compositionError || publicationError}
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
              children: entryEditor('proxy-groups'),
            },
            {
              key: 'providers',
              label: '规则集',
              disabled: !template,
              children: entryEditor('rule-providers'),
            },
            {
              key: 'routes',
              label: '路由',
              disabled: !template,
              children: entryEditor('rules'),
            },
            {
              key: 'load-balance',
              label: '负载均衡',
              disabled: !template,
              children: entryEditor('proxy-groups', true),
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
          <div
            className="subscription-source-toggle subscription-route-filters"
            role="group"
            aria-label="配置来源"
          >
            <Checkbox checked={showTemplate} onChange={(e) => setShowTemplate(e.target.checked)}>
              模板
            </Checkbox>
            <Checkbox checked={showCustom} onChange={(e) => setShowCustom(e.target.checked)}>
              自定义
            </Checkbox>
          </div>
        )}
      </TemplateWorkspace>
      {(error || yamlError || compositionError || publicationError) && (
        <Alert
          style={{ marginTop: 12 }}
          type="error"
          title={error || yamlError || compositionError || publicationError}
        />
      )}
      <div className="subscription-footer">
        <Space>
          <Button onClick={onClose}>关闭</Button>
          <Button
            type="primary"
            disabled={!chainValid || !!publicationError || !!yamlError || !!compositionError}
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
