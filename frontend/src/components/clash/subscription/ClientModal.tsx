import { useRef, useState } from 'react';
import { Alert, Button, ConfigProvider, Dropdown, Modal, Spin, Tabs, message, theme } from 'antd';
import { EditOutlined, PlusOutlined, UndoOutlined } from '@ant-design/icons';
import { useQueryClient } from '@tanstack/react-query';
import { keys } from '@/api/queryKeys';
import { useClashSubscription } from '@/api/queries/clash/subscription';
import { changeProfile } from '@/lib/clash/subscription/api';
import { RandomUtil } from '@/utils';
import { emptySubscription, type Profile } from '@/schemas/clash/subscription';
import SubscriptionEditor, {
  type EditorHandle,
  type SubscriptionClientProps as Props,
} from './SubscriptionEditor';
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
  const [profiles, setProfiles] = useState<DraftProfile[]>(() =>
    data.profiles.length
      ? data.profiles
      : [
          {
            ...structuredClone(emptySubscription),
            key: 'default',
            name: '新订阅',
            token: RandomUtil.randomSeq(20),
            isNew: true,
          },
        ],
  );
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
  const [saveError, setSaveError] = useState('');
  const cache = useQueryClient();
  const readers = useRef(new Map<string, EditorHandle>());
  const originals = useRef(new Map(profiles.map((p) => [p.key, p])));
  const [dirtyEditors, setDirtyEditors] = useState<Record<string, boolean>>({});
  const [versions, setVersions] = useState<Record<string, number>>({});
  function dirty(p: DraftProfile) {
    return (
      p.isNew ||
      dirtyEditors[p.key] ||
      p.pendingToken ||
      originals.current.get(p.key)?.name !== p.name
    );
  }
  function restore(p: DraftProfile) {
    const original = originals.current.get(p.key)!;
    setProfiles((all) => all.map((item) => (item.key === p.key ? original : item)));
    setVersions((all) => ({ ...all, [p.key]: (all[p.key] ?? 0) + 1 }));
    setDirtyEditors((all) => ({ ...all, [p.key]: false }));
  }
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
    const binding = mode === 'current' ? readers.current.get(active)?.read() : source;
    if (!source || !binding) {
      void message.error('请先修复当前配置');
      return;
    }
    const key = RandomUtil.randomUUID();
    const copy: DraftProfile = {
      ...structuredClone(binding),
      key,
      name: source.name + ' 副本',
      token: RandomUtil.randomSeq(20),
      updatedAt: 0,
      lastSyncAt: 0,
      isNew: true,
    };
    originals.current.set(key, copy);
    setProfiles((all) => [...all, copy]);
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
  async function save() {
    if (busy) return false;
    setBusy(true);
    setSaveError('');
    try {
      const targets = profiles.filter(dirty);
      if (!targets.length && removed.length) targets.push(draft);
      const prepared = [];
      for (const profile of targets) {
        try {
          const editor = readers.current.get(profile.key);
          if (!editor) throw Error('订阅编辑器尚未就绪');
          const pending = await editor.prepare();
          if (!pending) return false;
          prepared.push({ profile, ...pending });
        } catch (error) {
          throw Error(profile.name + '：' + String(error));
        }
      }
      let deletions = removed;
      for (const { profile, binding, rotate } of prepared) {
        try {
          const result = await changeProfile(
            clientId,
            {
              ...binding,
              key: profile.key,
              name: profile.name,
              token: rotate ? profile.pendingToken : profile.token,
            },
            profile.updatedAt,
            profile.isNew,
            deletions,
          );
          deletions = [];
          setRemoved([]);
          originals.current.set(profile.key, result);
          restore(result);
        } catch (error) {
          throw Error(profile.name + '：' + String(error));
        }
      }
      return true;
    } catch (error) {
      setSaveError(String(error));
      return false;
    } finally {
      setBusy(false);
      await cache.invalidateQueries({ queryKey: keys.clash.subscription(clientId) });
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
    <div inert={busy}>
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
            <span className={dirty(p) ? 'profile-unsaved' : undefined}>
              <span className="profile-label">
                <span>
                  {p.name} {active === p.key && <EditOutlined />}
                </span>
                {active === p.key && (
                  <input
                    aria-label="订阅名称"
                    key={versions[p.key] ?? 0}
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
              {dirty(p) && (
                <Button
                  size="small"
                  type="text"
                  aria-label={'撤销 ' + p.name + ' 的未保存修改'}
                  title="撤销此页签的未保存修改"
                  icon={<UndoOutlined />}
                  onKeyDown={(e) => e.stopPropagation()}
                  onClick={(e) => {
                    e.stopPropagation();
                    restore(p);
                  }}
                />
              )}
            </span>
          ),
        }))}
      />
      {saveError && <Alert type="error" title={saveError} />}
      {profiles
        .filter((p) => visited.has(p.key))
        .map((p) => {
          const token = p.pendingToken ?? p.token;
          const address =
            subscriptionURL &&
            token &&
            new URL(token, new URL(subscriptionURL, window.location.href)).href;
          return (
            <div key={p.key} hidden={active !== p.key}>
              <SubscriptionEditor
                client={client}
                subscriptionURL={subscriptionURL}
                profileURL={address || undefined}
                pendingRotation={!!p.pendingToken}
                onRotate={() => update(p.key, { pendingToken: RandomUtil.randomSeq(20) })}
                onClose={onClose}
                key={versions[p.key] ?? 0}
                profileName={p.name}
                onDirty={(value) =>
                  setDirtyEditors((all) =>
                    !!all[p.key] === value ? all : { ...all, [p.key]: value },
                  )
                }
                draftRef={(editor) => {
                  if (editor) readers.current.set(p.key, editor);
                  else readers.current.delete(p.key);
                }}
                data={{ ...data, binding: p }}
                onSave={save}
              />
            </div>
          );
        })}
    </div>
  );
}
