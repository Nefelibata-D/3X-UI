import { blacklistRuleKinds } from '@/lib/clash/subscription/loadBalance';
import { EditOutlined, CopyOutlined, DownloadOutlined, DeleteOutlined } from '@ant-design/icons';
import { Tag } from 'antd';
import './RuleFilePreview.css';

import { useLocation, useNavigate } from 'react-router';
import { Form, Upload, Layout, ConfigProvider, Flex, Spin } from 'antd';
import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import '@/styles/page-shell.css';
import Templates from './Templates';
import { useState, type UIEvent } from 'react';
import { useForm, useWatch } from 'react-hook-form';
import {
  useClashRuleSets,
  useSaveClashRuleSet,
  useDeleteClashRuleSet,
} from '@/api/queries/clash/subscription';
import {
  message,
  Button,
  Card,
  Checkbox,
  Empty,
  Input,
  Modal,
  Select,
  Popconfirm,
  Space,
  Tabs,
  Typography,
} from 'antd';
import { useAllSettings } from '@/api/queries/useAllSettings';
import { importRules } from '@/lib/clash/subscription/api';
import type { RuleSet as Row } from '@/schemas/clash/subscription';

const blank: Row = {
  id: 0,
  code: '',
  filename: '',
  name: '',
  routing: true,
  blacklist: false,
  rules: [],
};
const blacklistKinds = blacklistRuleKinds;
const kinds = [
  'DOMAIN',
  'DOMAIN-SUFFIX',
  'DOMAIN-KEYWORD',
  'DOMAIN-REGEX',
  'DOMAIN-WILDCARD',
  'GEOSITE',
  'GEOIP',
  'SRC-GEOIP',
  'IP-ASN',
  'SRC-IP-ASN',
  'IP-CIDR',
  'IP-CIDR6',
  'SRC-IP-CIDR',
  'IP-SUFFIX',
  'SRC-IP-SUFFIX',
  'SRC-PORT',
  'DST-PORT',
  'IN-PORT',
  'DSCP',
  'PROCESS-NAME',
  'PROCESS-PATH',
  'PROCESS-NAME-REGEX',
  'PROCESS-PATH-REGEX',
  'PROCESS-NAME-WILDCARD',
  'PROCESS-PATH-WILDCARD',
  'NETWORK',
  'UID',
  'IN-TYPE',
  'IN-USER',
  'IN-NAME',
  'REMATCH-NAME',
  'AND',
  'OR',
  'NOT',
];
function url(row: Row) {
  return new URL(
    (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '') +
      '/rulesets/' +
      row.code +
      '/' +
      encodeURIComponent(row.filename),
    window.location.origin,
  ).href;
}
const scrollVisibility = {
  onScroll(event: UIEvent<HTMLElement>) {
    event.currentTarget.toggleAttribute('data-scrolling', true);
  },
  onScrollEnd(event: UIEvent<HTMLElement>) {
    event.currentTarget.removeAttribute('data-scrolling');
  },
};
function Rules() {
  const list = useClashRuleSets();
  const saveMutation = useSaveClashRuleSet();
  const deleteMutation = useDeleteClashRuleSet();
  const form = useForm<Row>({ defaultValues: blank });
  const row = useWatch({ control: form.control }) as Row;
  const [editing, edit] = useState(false);
  const [entryTab, setEntryTab] = useState('entries');
  const [paste, setPaste] = useState('');
  const [saving, setSaving] = useState(false);
  const eligible = (rules: string[]) =>
    rules.every((r) => blacklistRuleKinds.includes(r.slice(0, r.indexOf(','))));
  function rulesChanged(rules: string[]) {
    if (form.getValues('blacklist') && !eligible(rules))
      throw new Error('黑名单只允许 DOMAIN / PROCESS 系列规则，请先取消黑名单用途。');
    form.setValue('rules', rules);
  }
  function open(item: Row) {
    form.reset(item);
    setEntryTab('entries');
    setPaste('');
    edit(true);
  }
  async function importContent(content: string) {
    rulesChanged([...form.getValues('rules'), ...(await importRules(content))]);
    setPaste('');
    setEntryTab('entries');
  }
  async function save() {
    setSaving(true);
    try {
      await saveMutation.mutateAsync(form.getValues());
      edit(false);
    } catch (e) {
      void message.error(String(e));
    } finally {
      setSaving(false);
    }
  }
  return (
    <div className="clash-rules-workspace">
      <div className="clash-rules-scroll" {...scrollVisibility}>
        {list.error && <Typography.Text type="danger">{list.error.message}</Typography.Text>}
        {list.isLoading ? (
          <Spin />
        ) : list.data?.length ? (
          <div className="clash-ruleset-grid">
            {list.data.map((r) => (
              <Card
                key={r.id}
                size="small"
                className="clash-ruleset-card"
                title={<Typography.Text strong>{r.name}</Typography.Text>}
                extra={
                  <Space size={2}>
                    <Button
                      type="text"
                      size="small"
                      aria-label="编辑"
                      title="编辑"
                      icon={<EditOutlined aria-hidden />}
                      onClick={() => open(r)}
                    />
                    <Button
                      type="text"
                      size="small"
                      aria-label="复制"
                      title="复制"
                      icon={<CopyOutlined aria-hidden />}
                      onClick={async () => {
                        try {
                          await navigator.clipboard.writeText(url(r));
                          message.success('链接已复制');
                        } catch {
                          message.error('复制失败');
                        }
                      }}
                    />
                    <Button
                      type="link"
                      size="small"
                      aria-label="下载"
                      title="下载"
                      icon={<DownloadOutlined aria-hidden />}
                      href={url(r)}
                      download={r.filename}
                    />
                    <Popconfirm
                      title={`删除规则集“${r.name}”？`}
                      description="删除后发布链接将失效。"
                      okText="删除"
                      cancelText="取消"
                      okButtonProps={{ danger: true }}
                      onConfirm={async () => {
                        try {
                          await deleteMutation.mutateAsync(r.id);
                          message.success('规则集已删除');
                        } catch (e) {
                          message.error(String(e));
                        }
                      }}
                    >
                      <Button
                        type="text"
                        size="small"
                        danger
                        aria-label="删除"
                        title="删除"
                        icon={<DeleteOutlined aria-hidden />}
                        loading={deleteMutation.isPending && deleteMutation.variables === r.id}
                        disabled={deleteMutation.isPending}
                      />
                    </Popconfirm>
                  </Space>
                }
              >
                <div className="clash-ruleset-rule-list" {...scrollVisibility}>
                  {r.rules.map((rule, index) => (
                    <div className="clash-ruleset-rule" key={`${r.id}-${index}`}>
                      <span>{index + 1}</span>
                      <code>
                        <b>{rule.split(',', 1)[0]}</b>
                        {rule.slice(rule.split(',', 1)[0].length)}
                      </code>
                    </div>
                  ))}
                </div>
                <div className="clash-ruleset-card-footer">
                  <Typography.Text type="secondary">
                    {r.rules.length} 条规则 ·{' '}
                    {r.updatedAt
                      ? new Date(r.updatedAt).toLocaleString('zh-CN', { hour12: false })
                      : '未更新'}
                  </Typography.Text>
                  <Space size={10}>
                    <Tag color={r.blacklist ? 'purple' : 'blue'}>
                      {r.routing && r.blacklist ? '路由 / 黑名单' : r.routing ? '路由' : '黑名单'}
                    </Tag>
                    <Tag color="cyan">Classical</Tag>
                  </Space>
                </div>
              </Card>
            ))}
          </div>
        ) : (
          <Empty description="还没有规则集" />
        )}
      </div>
      <Button className="clash-rules-create" type="primary" onClick={() => open(blank)}>
        新建规则集
      </Button>
      <Modal
        title={row.id ? '编辑规则集' : '新建规则集'}
        open={editing}
        centered
        className="clash-rule-editor"
        onCancel={() => edit(false)}
        onOk={save}
        width={680}
        okText="保存"
        confirmLoading={saving}
        okButtonProps={{
          disabled:
            !row.name.trim() ||
            !row.rules.length ||
            (!row.routing && !row.blacklist) ||
            !!paste.trim(),
        }}
      >
        <Form layout="vertical">
          <Flex wrap gap="0 24px">
            <Flex vertical style={{ flex: '7 1 240px', minWidth: 0 }}>
              <Form.Item label="规则集名称" layout="vertical" required>
                <Input
                  aria-label="规则集名称"
                  placeholder="规则集名称"
                  value={row.name}
                  onChange={(e) => form.setValue('name', e.target.value)}
                />
              </Form.Item>
            </Flex>
            <Flex vertical style={{ flex: '5 1 170px', minWidth: 0 }}>
              <Form.Item label="用途" tooltip="黑名单支持 DOMAIN / PROCESS 系列规则。">
                <Space>
                  <Checkbox
                    checked={row.routing}
                    onChange={(e) => form.setValue('routing', e.target.checked)}
                  >
                    路由
                  </Checkbox>
                  <Checkbox
                    disabled={!eligible(row.rules)}
                    checked={row.blacklist}
                    onChange={(e) => form.setValue('blacklist', e.target.checked)}
                  >
                    黑名单
                  </Checkbox>
                </Space>
              </Form.Item>
            </Flex>
          </Flex>
          <Tabs
            activeKey={entryTab}
            onChange={setEntryTab}
            items={[
              {
                key: 'entries',
                label: '规则条目（' + row.rules.length + '）',
                children: (
                  <div className="clash-rule-viewport">
                    <Flex vertical gap="small" className="clash-rule-rows" {...scrollVisibility}>
                      <Flex vertical gap="small">
                        {row.rules.map((line, i) => {
                          const split = line.indexOf(',');
                          const kind = line.slice(0, split),
                            value = line.slice(split + 1);
                          const update = (next: string) =>
                            rulesChanged(row.rules.map((old, n) => (n === i ? next : old)));
                          return (
                            <Space.Compact key={i} style={{ width: '100%' }}>
                              <Select
                                aria-label="匹配类型"
                                value={kind}
                                options={(row.blacklist ? blacklistKinds : kinds).map((value) => ({
                                  value,
                                }))}
                                onChange={(v) => update(v + ',' + value)}
                                style={{ minWidth: 155 }}
                              />
                              <Input
                                className="panel-code-input"
                                aria-label="匹配内容"
                                value={value}
                                onChange={(e) => update(kind + ',' + e.target.value)}
                              />
                              <Button
                                onClick={() => rulesChanged(row.rules.filter((_, n) => n !== i))}
                              >
                                移除
                              </Button>
                            </Space.Compact>
                          );
                        })}
                      </Flex>
                      <Button
                        type="dashed"
                        block
                        onClick={() => rulesChanged([...row.rules, 'DOMAIN,'])}
                      >
                        添加规则条目
                      </Button>
                    </Flex>
                  </div>
                ),
              },
              {
                key: 'import',
                label: '批量导入',
                children: (
                  <Flex vertical gap="small">
                    <Input.TextArea
                      className="panel-code-input"
                      rows={5}
                      aria-label="导入规则"
                      placeholder="粘贴 payload YAML 或每行一条 classical 规则"
                      value={paste}
                      onChange={(e) => setPaste(e.target.value)}
                    />
                    <Space>
                      <Button
                        disabled={!paste.trim()}
                        onClick={() =>
                          void importContent(paste).catch((e) => message.error(String(e)))
                        }
                      >
                        识别并追加
                      </Button>
                      <Upload
                        accept=".yaml,.yml,.txt,.list"
                        showUploadList={false}
                        beforeUpload={(file) => {
                          void file
                            .text()
                            .then(importContent)
                            .catch((err) => message.error(String(err)));
                          return false;
                        }}
                      >
                        <Button>从文件导入</Button>
                      </Upload>
                    </Space>
                  </Flex>
                ),
              },
            ]}
          />
        </Form>
      </Modal>
    </div>
  );
}
export default function ClashSubscriptionsPage() {
  const { hash } = useLocation();
  const navigate = useNavigate();
  const { allSetting, fetched } = useAllSettings();
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout
        className={[
          'nodes-page',
          hash !== '#templates' && 'clash-rules-page',
          isDark && 'is-dark',
          isUltra && 'is-ultra',
        ]
          .filter(Boolean)
          .join(' ')}
      >
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <Card>
              {!fetched ? (
                <Spin />
              ) : !allSetting.subClashEnable ? (
                '请先在面板设置中开启 Clash / Mihomo 订阅。'
              ) : (
                <Tabs
                  activeKey={hash === '#templates' ? 'templates' : 'rules'}
                  onChange={(key) => navigate({ hash: key })}
                  items={[
                    { key: 'rules', label: '规则集', children: <Rules /> },
                    { key: 'templates', label: 'YAML 模板', children: <Templates /> },
                  ]}
                />
              )}
            </Card>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
