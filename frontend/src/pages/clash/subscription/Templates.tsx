import {
  message,
  Alert,
  Button,
  Card,
  Flex,
  Input,
  Modal,
  Popconfirm,
  Select,
  Space,
  Table,
  Tag,
  Tabs,
  Typography,
  Upload,
} from 'antd';
import {
  CopyOutlined,
  EditOutlined,
  DeleteOutlined,
  PlusOutlined,
  UploadOutlined,
} from '@ant-design/icons';
import {
  useClashTemplates,
  useClashRuleSets,
  useClashTemplateMutations,
} from '@/api/queries/clash/subscription';
import type { Template } from '@/schemas/clash/subscription';
import { useForm, useWatch } from 'react-hook-form';
import { useMemo, useState } from 'react';
import { isMap, isSeq, parseDocument } from 'yaml';
import { z } from 'zod';
import LoadBalance from '@/components/clash/subscription/LoadBalance';
import { validateTemplate } from '@/lib/clash/subscription/template/validation';
import TemplateWorkspace from '@/components/clash/subscription/TemplateWorkspace';
import NodeGroups from '@/components/clash/subscription/NodeGroups';
import TemplateInsert from '@/components/clash/subscription/TemplateInsert';
import '@/components/clash/subscription/Templates.css';

const empty: Template = {
  id: 0,
  name: '',
  groups: [],
  injections: { groups: [], routes: [], blacklists: [] },
  content: `mixed-port: 7890
allow-lan: false
mode: rule
log-level: info

proxies: []

proxy-groups:
  - name: 节点选择
    type: select
    proxies: []

rules:
  - MATCH,节点选择
`,
};
export default function Templates() {
  const mutations = useClashTemplateMutations();
  const form = useForm<Template>({ defaultValues: empty });
  const draft = useWatch({ control: form.control }) as Template;
  const [mode, setMode] = useState<'edit' | 'view'>();
  const [chainValid, setChainValid] = useState(true);
  const [saving, setSaving] = useState(false);
  const query = useClashTemplates();
  const summaries = useMemo(
    () =>
      new Map(
        (query.data ?? []).map((row) => {
          const doc = parseDocument(row.content);
          if (doc.errors.length) return [row.id, undefined] as const;
          const providers = doc.get('rule-providers');
          const routes = doc.get('rules');
          const proxies = doc.get('proxies');
          const groupList = doc.get('proxy-groups');
          const groups = isSeq(groupList) ? groupList.items.filter(isMap) : [];
          return [
            row.id,
            {
              providers: isMap(providers) ? providers.items.length : 0,
              routes: isSeq(routes) ? routes.items.length : 0,
              chain:
                isSeq(proxies) &&
                proxies.items.some(
                  (p) =>
                    isMap(p) &&
                    typeof p.get('dialer-proxy') === 'string' &&
                    String(p.get('dialer-proxy')).trim() !== '',
                ),
              balance: groups
                .filter((g) => g.get('type') === 'load-balance')
                .map((g) => String(g.get('name'))),
            },
          ] as const;
        }),
      ),
    [query.data],
  );
  const colors = ['blue', 'purple', 'cyan', 'geekblue', 'magenta', 'green', 'orange'];
  async function removeTemplate(id: number) {
    await mutations.remove.mutateAsync(id);
    void message.success('模板已删除');
  }
  const rules = useClashRuleSets(!!mode);
  const parsed = useMemo(() => {
    try {
      const doc = parseDocument(draft.content);
      if (doc.errors.length) throw new Error(doc.errors[0].message);
      const config = z
        .object({
          'proxy-groups': z
            .array(z.object({ name: z.string().min(1), type: z.string().min(1) }))
            .optional(),
        })
        .passthrough()
        .parse(doc.toJS());
      const groups = config['proxy-groups'] ?? [];
      if (new Set(groups.map((g) => g.name)).size !== groups.length)
        throw new Error('策略组名称重复');
      return { groups, error: '' };
    } catch (e) {
      return { groups: [], error: String(e) };
    }
  }, [draft.content]);
  const readonly = mode === 'view';
  const targetOptions = [
    'DIRECT',
    'REJECT',
    'REJECT-DROP',
    'PASS',
    ...parsed.groups.map((g) => g.name),
  ].map((value) => ({
    value,
    label: value,
  }));
  const dangling =
    draft.injections.groups.some((b) => !parsed.groups.some((g) => g.name === b.group)) ||
    draft.injections.routes.some((b) => !targetOptions.some((t) => t.value === b.target));
  function insertPanel(tab: 'providers' | 'routes' | 'dialer') {
    return (
      <TemplateInsert
        key={mode + String(draft.id) + tab}
        onValidityChange={setChainValid}
        tab={tab}
        entryClassName={tab === 'providers' ? () => 'template-entry-card' : undefined}
        content={draft.content}
        onChange={(v) => form.setValue('content', v)}
        groups={parsed.groups.map((g) => g.name)}
        rules={rules.data ?? []}
        readonly={readonly}
      />
    );
  }
  function checkTemplate() {
    const report = validateTemplate(draft.content);
    void message.open({
      key: 'template-validation',
      type: report.errors.length ? 'error' : report.pending.length ? 'warning' : 'success',
      duration: 8,
      content: (
        <div style={{ maxHeight: '40vh', maxWidth: '80vw', overflowY: 'auto', textAlign: 'left' }}>
          <strong>
            {report.errors.length
              ? '模板校验发现问题'
              : report.pending.length
                ? '部分策略组待注入'
                : '模板静态校验通过'}
          </strong>
          {report.errors.map((text, index) => (
            <div key={'e' + index}>{text}</div>
          ))}
          {!!report.pending.length && (
            <div>待客户端注入：{report.pending.join('、')}。下发前必须完成注入。</div>
          )}
          {report.warnings.map((text, index) => (
            <div key={'w' + index}>{text}</div>
          ))}
        </div>
      ),
    });
  }
  function open(value: Template, next: 'edit' | 'view') {
    setChainValid(true);
    form.reset(structuredClone(value));
    setMode(next);
  }
  async function save() {
    const report = validateTemplate(form.getValues('content'));
    if (report.errors.length) {
      checkTemplate();
      return;
    }
    setSaving(true);
    try {
      await mutations.save.mutateAsync(form.getValues());
      setMode(undefined);
    } catch (e) {
      void message.error(String(e));
    } finally {
      setSaving(false);
    }
  }
  return (
    <Space orientation="vertical" style={{ width: '100%' }}>
      <Flex justify="space-between" align="center">
        <div>
          <Typography.Title level={4} style={{ margin: 0 }}>
            订阅模板
          </Typography.Title>
        </div>
        <Button
          type="primary"
          icon={<PlusOutlined aria-hidden />}
          onClick={() => open(empty, 'edit')}
        >
          新建模板
        </Button>
      </Flex>
      {query.error && <Alert type="error" title={query.error.message} />}
      <Table<Template>
        rowKey="id"
        scroll={{ x: 1100 }}
        loading={query.isLoading}
        dataSource={query.data}
        columns={[
          { title: '模板名称', dataIndex: 'name' },
          {
            title: '策略组',
            render: (_, r) => (
              <Space wrap>
                {r.groups.map((g, index) => (
                  <Tag color={colors[index % colors.length]} key={g}>
                    {g}
                  </Tag>
                ))}
              </Space>
            ),
          },
          {
            title: '规则集数目',
            align: 'center',
            render: (_, r) => summaries.get(r.id)?.providers ?? '—',
          },
          {
            title: '路由数目',
            align: 'center',
            render: (_, r) => summaries.get(r.id)?.routes ?? '—',
          },
          {
            title: '链式代理配置',
            align: 'center',
            render: (_, r) => {
              const summary = summaries.get(r.id);
              return summary ? (summary.chain ? '是' : '否') : '—';
            },
          },
          {
            title: '负载均衡代理组',
            render: (_, r) => {
              const summary = summaries.get(r.id);
              return summary?.balance.length ? (
                <Space wrap>
                  {summary.balance.map((name) => (
                    <Tag
                      key={name}
                      color={colors[Math.max(0, r.groups.indexOf(name)) % colors.length]}
                    >
                      {name}
                    </Tag>
                  ))}
                </Space>
              ) : (
                '—'
              );
            },
          },
          {
            title: '操作',
            render: (_, r) => (
              <Space>
                <Button
                  type="text"
                  icon={<EditOutlined aria-hidden />}
                  onClick={() => open(r, 'edit')}
                >
                  编辑
                </Button>
                <Button
                  type="text"
                  icon={<CopyOutlined aria-hidden />}
                  onClick={() =>
                    open(
                      {
                        ...r,
                        id: 0,
                        name: r.name + ' 副本',
                        injections: { ...r.injections, groups: [] },
                      },
                      'edit',
                    )
                  }
                >
                  复制
                </Button>
                <Popconfirm
                  title={`删除模板“${r.name}”？`}
                  description="删除后无法恢复。"
                  okText="删除"
                  cancelText="取消"
                  okButtonProps={{ danger: true }}
                  onConfirm={() => removeTemplate(r.id)}
                >
                  <Button type="text" danger icon={<DeleteOutlined aria-hidden />}>
                    删除
                  </Button>
                </Popconfirm>
              </Space>
            ),
          },
        ]}
      />
      <Modal
        title={readonly ? '查看模板' : draft.id ? '编辑模板' : '新建模板'}
        width={1240}
        centered
        open={!!mode}
        onCancel={() => setMode(undefined)}
        onOk={save}
        confirmLoading={saving}
        footer={(_, { CancelBtn, OkBtn }) => (
          <Space>
            {readonly ? <Button onClick={() => setMode(undefined)}>关闭</Button> : <CancelBtn />}
            <Button onClick={() => checkTemplate()}>校验模板</Button>
            {!readonly && <OkBtn />}
          </Space>
        )}
        okText="保存模板"
        okButtonProps={{
          disabled: !draft.name.trim() || !!parsed.error || dangling || !chainValid,
        }}
      >
        <Flex gap={12} align="center" style={{ marginBottom: 16 }}>
          <Input
            aria-label="模板名称"
            placeholder="模板名称，例如日常使用"
            value={draft.name}
            readOnly={readonly}
            onChange={(e) => form.setValue('name', e.target.value)}
          />
          {!readonly && (
            <Upload
              accept=".yaml,.yml"
              showUploadList={false}
              beforeUpload={(file) => {
                void file
                  .text()
                  .then((v) => form.setValue('content', v))
                  .catch((e) => message.error(String(e)));
                return false;
              }}
            >
              <Button icon={<UploadOutlined aria-hidden />}>导入 YAML</Button>
            </Upload>
          )}
        </Flex>
        <TemplateWorkspace
          content={draft.content}
          onChange={(v) => form.setValue('content', v)}
          readonly={readonly}
          error={parsed.error}
          groupCount={parsed.groups.length}
        >
          {rules.error && <Alert type="error" title={String(rules.error)} />}
          <Tabs
            items={[
              {
                key: 'groups',
                label: '代理组',
                children: (
                  <Space orientation="vertical" style={{ width: '100%' }}>
                    <NodeGroups
                      entryClassName={() => 'template-entry-card'}
                      key={mode + String(draft.id)}
                      readonly={readonly}
                      invalid={!!parsed.error}
                      content={draft.content}
                      groups={parsed.groups}
                      bindings={draft.injections.groups}
                      change={(content, bindings, removed, replacement) => {
                        form.setValue('content', content);
                        form.setValue('injections.groups', bindings);
                        if (removed && replacement)
                          form.setValue(
                            'injections.routes',
                            draft.injections.routes.map((r) =>
                              r.target === removed ? { ...r, target: replacement } : r,
                            ),
                          );
                      }}
                    />
                    {dangling && (
                      <Alert
                        type="error"
                        title="策略组已删除或改名，请恢复组名或清除旧注入配置。"
                        action={
                          !readonly && (
                            <Button
                              onClick={() => {
                                form.setValue(
                                  'injections.groups',
                                  draft.injections.groups.filter((b) =>
                                    parsed.groups.some((g) => g.name === b.group),
                                  ),
                                );
                                form.setValue(
                                  'injections.routes',
                                  draft.injections.routes.filter((b) =>
                                    targetOptions.some((t) => t.value === b.target),
                                  ),
                                );
                              }}
                            >
                              清除失效引用
                            </Button>
                          )
                        }
                      />
                    )}
                  </Space>
                ),
              },
              { key: 'providers', label: '规则集', children: insertPanel('providers') },
              {
                key: 'routes',
                label: '路由',
                children: (
                  <Space orientation="vertical" style={{ width: '100%' }}>
                    {insertPanel('routes')}
                    {draft.injections.routes.length > 0 && (
                      <Typography.Text type="secondary">已有模板路由绑定</Typography.Text>
                    )}
                    {draft.injections.routes.map((b, i) => (
                      <Card size="small" key={i}>
                        <Space orientation="vertical" style={{ width: '100%' }}>
                          <Select
                            aria-label="路由规则集"
                            placeholder="选择路由规则集"
                            style={{ width: '100%' }}
                            disabled={readonly}
                            value={b.ruleSetId || undefined}
                            options={rules.data
                              ?.filter((r) => r.routing)
                              .map((r) => ({ value: r.id, label: r.name }))}
                            onChange={(v) => form.setValue(`injections.routes.${i}.ruleSetId`, v)}
                          />
                          <Select
                            aria-label="路由目标"
                            style={{ width: '100%' }}
                            disabled={readonly}
                            value={b.target}
                            options={targetOptions}
                            onChange={(v) => form.setValue(`injections.routes.${i}.target`, v)}
                          />
                          {!readonly && (
                            <Button
                              type="text"
                              danger
                              onClick={() =>
                                form.setValue(
                                  'injections.routes',
                                  draft.injections.routes.filter((_, n) => n !== i),
                                )
                              }
                            >
                              移除
                            </Button>
                          )}
                        </Space>
                      </Card>
                    ))}
                  </Space>
                ),
              },
              {
                key: 'load-balance',
                label: '负载均衡',
                children: (
                  <>
                    {!!draft.injections.blacklists.length && (
                      <Alert
                        type="warning"
                        title="此模板含旧版黑名单绑定，请根据原有配置在代理组下重新设置。旧数据已保留。"
                      />
                    )}
                    <LoadBalance
                      content={draft.content}
                      readonly={readonly}
                      rules={rules.data ?? []}
                      onChange={(value) => form.setValue('content', value)}
                    />
                  </>
                ),
              },
              { key: 'dialer', label: '链式代理', children: insertPanel('dialer') },
            ]}
          />
        </TemplateWorkspace>
        {parsed.error && <Alert style={{ marginTop: 12 }} type="error" title={parsed.error} />}
      </Modal>
    </Space>
  );
}
