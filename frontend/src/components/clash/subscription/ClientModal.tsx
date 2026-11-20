import { useRef, useState } from 'react';
import { Alert, Button, ConfigProvider, Dropdown, Modal, Spin, Tabs, message, theme } from 'antd';
import { EditOutlined, PlusOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useClashSubscription } from '@/api/queries/clash/subscription';
import { changeProfile } from '@/lib/clash/subscription/api';
import { RandomUtil } from '@/utils';
import { emptySubscription, type Profile, type Subscription } from '@/schemas/clash/subscription';
import SubscriptionEditor, { type SubscriptionClientProps as Props } from './SubscriptionEditor';
import './Templates.css';
import './ClientModal.css';

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
  const [profiles, setProfiles] = useState<DraftProfile[]>(() => data.profiles.length ? data.profiles : [{
    ...structuredClone(emptySubscription), key: 'default', name: '新订阅', token: RandomUtil.randomSeq(20), isNew: true,
  }]);
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
                profileURL={address || undefined}
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
