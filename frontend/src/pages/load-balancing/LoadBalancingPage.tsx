import { useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  ConfigProvider,
  Layout,
  Alert,
  Button,
  DatePicker,
  Card,
  Col,
  Empty,
  InputNumber,
  Progress,
  Row,
  Select,
  Space,
  Statistic,
  Tabs,
  Table,
  Tag,
  Tooltip,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import dayjs from 'dayjs';
import { InfoCircleOutlined, ReloadOutlined, SyncOutlined, UndoOutlined } from '@ant-design/icons';

import AppSidebar from '@/layouts/AppSidebar';
import { useTheme } from '@/hooks/useTheme';
import { keys } from '@/api/queryKeys';
import { useClashOverviewQuery } from '@/api/queries/useClashOverviewQuery';
import {
  normalizePreview,
  gbToBytes,
  bytesToGB,
  resetClashPolicy,
  saveClashWeights,
  syncClashPolicies,
} from '@/lib/clash/api';
import type { NodeOverview, PolicyOverview } from '@/schemas/clash';
import { SizeFormatter } from '@/utils';
import './LoadBalancingPage.css';

function formatTime(unix: number | undefined): string {
  return unix && unix > 0 ? dayjs(unix * 1000).format('YYYY-MM-DD HH:mm') : '-';
}

function clientKey(policy: PolicyOverview): string {
  return `${policy.masterGuid}|${policy.clientId}`;
}

function nodeStatusLabel(node: NodeOverview): string {
  if (!node.enabled) return '已禁用';
  switch (node.status) {
    case 'online':
      return '在线';
    case 'offline':
      return '离线';
    case 'unknown':
      return '未知';
    default:
      return '-';
  }
}

const EMPTY_POLICIES: PolicyOverview[] = [];

export default function LoadBalancingPage() {
  const { isDark, isUltra, antdThemeConfig } = useTheme();
  const queryClient = useQueryClient();
  const query = useClashOverviewQuery();
  const data = query.data;
  const [edits, setEdits] = useState<Record<
    string,
    Pick<NodeOverview, 'defaultWeight' | 'capacityBytes' | 'resetSchedule'>
  > | null>(null);
  const [resetTarget, setResetTarget] = useState('');
  const [activeTab, setActiveTab] = useState('nodes');
  const [clientFilter, setClientFilter] = useState<string>();
  const [nodeFilter, setNodeFilter] = useState<string>();
  const policies = data?.policies ?? EMPTY_POLICIES;

  const derivedDrafts = useMemo(() => {
    const next: Record<
      string,
      Pick<NodeOverview, 'defaultWeight' | 'capacityBytes' | 'resetSchedule'>
    > = {};
    for (const node of data?.nodes ?? [])
      next[node.key] = {
        defaultWeight: node.defaultWeight,
        capacityBytes: node.capacityBytes,
        resetSchedule: node.resetSchedule,
      };
    return next;
  }, [data]);
  const drafts = edits ?? derivedDrafts;
  const dirty = edits !== null;

  const activePolicies = useMemo(
    () => (data?.policies ?? []).filter((policy) => !policy.remove),
    [data?.policies],
  );
  const statistics = useMemo(
    () => ({
      clients: new Set(activePolicies.map(clientKey)).size,
      onlineNodes: (data?.nodes ?? []).filter((node) => node.enabled && node.status === 'online')
        .length,
      normal: activePolicies.filter((policy) => policy.state === 'normal').length,
      grace: activePolicies.filter((policy) => policy.state === 'grace').length,
      disabled: activePolicies.filter((policy) => policy.state === 'disabled').length,
      unapplied: activePolicies.filter((policy) => policy.status !== 'applied').length,
    }),
    [activePolicies, data?.nodes],
  );

  const clientOptions = useMemo(() => {
    const clients = new Map<string, string>();
    for (const policy of policies) {
      clients.set(
        clientKey(policy),
        `${policy.clientEmail || '未命名客户端'} · #${policy.clientId}`,
      );
    }
    return [...clients].map(([value, label]) => ({ value, label }));
  }, [policies]);

  const filteredPolicies = useMemo(
    () =>
      policies.filter(
        (policy) =>
          (!clientFilter || clientKey(policy) === clientFilter) &&
          (!nodeFilter || policy.nodeKey === nodeFilter),
      ),
    [policies, clientFilter, nodeFilter],
  );

  const preview = useMemo(() => {
    if (!data) return {} as Record<string, number>;
    const entries = data.nodes
      .filter((node) => node.enabled)
      .map((node) => ({ key: node.key, weight: drafts[node.key]?.defaultWeight ?? 0 }))
      .filter((entry) => entry.weight > 0);
    const out: Record<string, number> = {};
    for (const row of normalizePreview(entries)) out[row.key] = row.ratio;
    return out;
  }, [data, drafts]);

  const saveMutation = useMutation({
    mutationFn: saveClashWeights,
    onSuccess: (overview) => {
      queryClient.setQueryData(keys.clash.overview(), overview);
      setEdits(null);
      queryClient.invalidateQueries({ queryKey: ['clash', 'client'] });
      message.success('节点配置已保存');
    },
    onError: (err: Error) => message.error(err.message),
  });

  const syncMutation = useMutation({
    mutationFn: syncClashPolicies,
    onSuccess: async () => {
      message.success('策略同步完成。');
      await query.refetch();
    },
    onError: (err: Error) => message.error(err.message),
  });

  const resetMutation = useMutation({
    mutationFn: ({ nodeKey, clientId }: { nodeKey: string; clientId: number }) =>
      resetClashPolicy(nodeKey, clientId),
    onSuccess: async () => {
      setResetTarget('');
      message.success('重置已排队，等待子节点应用。');
      await query.refetch();
    },
    onError: (err: Error) => {
      setResetTarget('');
      message.error(err.message);
    },
  });

  const onSave = () => {
    if (!data) return;
    const items = data.nodes.map((node) => ({
      nodeKey: node.key,
      ...drafts[node.key],
    }));
    saveMutation.mutate(items);
  };

  const stateLabel = (state: string | undefined) => {
    switch (state) {
      case 'normal':
        return '正常';
      case 'grace':
        return '宽限';
      case 'disabled':
        return '停用';
      case 'removed':
        return '已移除';
      case '':
        return '待应用';
      default:
        return state || '-';
    }
  };

  const stateColor = (state: string | undefined): string => {
    switch (state) {
      case 'normal':
        return 'green';
      case 'grace':
        return 'orange';
      case 'disabled':
        return 'red';
      case 'removed':
        return 'default';
      default:
        return 'blue';
    }
  };

  const applyLabel = (status: string | undefined) => {
    switch (status) {
      case 'applied':
        return '已应用';
      case 'pending':
        return '待应用';
      case 'offline':
        return '离线';
      case 'stale':
        return '版本冲突';
      case 'removed':
        return '已移除';
      case 'error':
        return '错误';
      default:
        return status || '-';
    }
  };

  const applyColor = (status: string | undefined): string => {
    switch (status) {
      case 'applied':
        return 'green';
      case 'pending':
        return 'blue';
      case 'offline':
      case 'stale':
        return 'orange';
      case 'error':
        return 'red';
      default:
        return 'default';
    }
  };

  const nodeColumns: ColumnsType<NodeOverview> = [
    {
      title: '出口',
      dataIndex: 'name',
      key: 'name',
      width: '14%',
      align: 'center',
      render: (_: string, node) => (
        <Space>
          <Typography.Text strong>{node.isLocal ? '本机' : node.name}</Typography.Text>
          {node.isLocal && <Tag color="blue">{'本机'}</Tag>}
        </Space>
      ),
    },
    {
      title: '状态',
      key: 'status',
      width: '9%',
      align: 'center',
      render: (_, node) => (
        <Tag color={!node.enabled ? 'red' : node.status === 'online' ? 'green' : 'default'}>
          {nodeStatusLabel(node)}
        </Tag>
      ),
    },
    {
      title: '节点身份',
      dataIndex: 'guid',
      key: 'guid',
      width: '13%',
      align: 'center',
      render: (guid: string) =>
        guid ? (
          <Typography.Text code copyable={{ text: guid }}>
            {guid.slice(0, 12)}
          </Typography.Text>
        ) : (
          <Typography.Text type="warning">{'等待节点心跳'}</Typography.Text>
        ),
    },
    {
      title: '总额度（GB）',
      key: 'capacity',
      width: '13%',
      align: 'center',
      render: (_, node) => (
        <InputNumber
          className="lb-number-input"
          min={0}
          precision={2}
          aria-label={`${node.name} 总额度`}
          value={bytesToGB(drafts[node.key]?.capacityBytes)}
          onChange={(value) =>
            setEdits({
              ...drafts,
              [node.key]: { ...drafts[node.key], capacityBytes: gbToBytes(value) },
            })
          }
        />
      ),
    },
    {
      title: '已分配 / 可分配',
      key: 'allocation',
      width: '17%',
      align: 'center',
      render: (_, node) => {
        const total = drafts[node.key]?.capacityBytes ?? 0;
        const over = total > 0 && node.allocatedBytes > total;
        return (
          <Typography.Text type={over ? 'danger' : undefined}>
            {SizeFormatter.sizeFormat(node.allocatedBytes)} /{' '}
            {total > 0 ? SizeFormatter.sizeFormat(total - node.allocatedBytes) : '不限'}
          </Typography.Text>
        );
      },
    },
    {
      title: '每月重置日期和时间',
      key: 'resetAt',
      width: '17%',
      align: 'center',
      render: (_, node) => {
        const schedule = drafts[node.key]?.resetSchedule;
        const value = dayjs()
          .date(schedule?.day || 1)
          .hour(Math.floor((schedule?.minute ?? 0) / 60))
          .minute((schedule?.minute ?? 0) % 60)
          .second(0);
        return (
          <DatePicker
            className="lb-reset-picker"
            showTime={{ format: 'HH:mm', showSecond: false }}
            format="D 日 HH:mm"
            value={value}
            aria-label={`${node.name} 每月重置日期和时间`}
            onChange={(date) => {
              if (!date) return;
              setEdits({
                ...drafts,
                [node.key]: {
                  ...drafts[node.key],
                  resetSchedule: {
                    ...drafts[node.key].resetSchedule,
                    day: date.date(),
                    minute: date.hour() * 60 + date.minute(),
                  },
                },
              });
            }}
          />
        );
      },
    },
    {
      title: '默认权重',
      key: 'weight',
      width: '9%',
      align: 'center',
      render: (_, node) => (
        <InputNumber
          className="lb-number-input"
          min={0}
          precision={0}
          aria-label={`${node.name} 默认权重`}
          value={drafts[node.key]?.defaultWeight ?? 0}
          onChange={(value) =>
            setEdits((prev) => ({
              ...(prev ?? derivedDrafts),
              [node.key]: { ...drafts[node.key], defaultWeight: Number(value) || 0 },
            }))
          }
        />
      ),
    },
    {
      title: '默认比例',
      key: 'ratio',
      width: '8%',
      align: 'center',
      render: (_, node) =>
        `${(((dirty ? preview[node.key] : node.defaultRatio) || 0) * 100).toFixed(1)}%`,
    },
  ];

  const policyColumns: ColumnsType<PolicyOverview> = [
    {
      title: '出口 / 客户端',
      key: 'node',
      width: '14%',
      align: 'center',
      render: (_, policy) => (
        <Space className="lb-client-node" direction="vertical" align="center" size={0}>
          <Typography.Text>
            {policy.nodeKey === 'local' ? '本机' : policy.nodeName || policy.nodeKey}
          </Typography.Text>
          <Typography.Text
            type="secondary"
            ellipsis={{ tooltip: policy.clientEmail || `#${policy.clientId}` }}
          >
            {policy.clientEmail || '-'} · #{policy.clientId}
          </Typography.Text>
          {policy.remote && <Tag color="purple">{'来自主控'}</Tag>}
        </Space>
      ),
    },
    {
      title: (
        <Tooltip title="正常：额度内；宽限：额度已用完，仍在宽限期；停用：宽限结束后停止该节点流量；待应用：暂无运行状态回报；已移除：策略正在移除或已移除。下发情况见“下发状态”。">
          <span>
            状态 <InfoCircleOutlined aria-hidden />
          </span>
        </Tooltip>
      ),
      key: 'state',
      width: '8%',
      align: 'center',
      render: (_, policy) => (
        <Tag color={stateColor(policy.state)}>
          {policy.remove ? '已移除' : stateLabel(policy.state)}
        </Tag>
      ),
    },
    {
      title: (
        <Tooltip title="已用量 = 上传 + 下载；限额 = 此客户端分配到该出口的额度。进度按已用量 ÷ 限额计算，达到 100% 后数字仍显示实际用量；∞ 表示不限。">
          <span>
            用量 <InfoCircleOutlined aria-hidden />
          </span>
        </Tooltip>
      ),
      key: 'usage',
      width: '15%',
      align: 'center',
      render: (_, policy) => {
        const total = policy.usageUp + policy.usageDown;
        const percent =
          policy.budgetBytes > 0 ? Math.min(100, (total / policy.budgetBytes) * 100) : 0;
        return (
          <Space direction="vertical" size={2} style={{ width: '100%' }}>
            {policy.budgetBytes > 0 ? (
              <Progress
                percent={Number(percent.toFixed(1))}
                size="small"
                strokeColor={
                  policy.state === 'disabled'
                    ? '#cf1322'
                    : policy.state === 'grace'
                      ? '#d46b08'
                      : undefined
                }
              />
            ) : (
              <Typography.Text type="secondary">{'不限额度'}</Typography.Text>
            )}
            <Typography.Text type="secondary">
              {SizeFormatter.sizeFormat(total)} /{' '}
              {policy.budgetBytes > 0 ? SizeFormatter.sizeFormat(policy.budgetBytes) : '∞'}
            </Typography.Text>
          </Space>
        );
      },
    },
    {
      title: '周期结束',
      dataIndex: 'periodEnd',
      key: 'period',
      width: '12%',
      align: 'center',
      render: (value: number) => formatTime(value),
    },
    {
      title: '受管入站',
      key: 'inbounds',
      width: '21%',
      align: 'center',
      render: (_, policy) =>
        policy.managedInbounds.length > 0 ? (
          <div className="lb-managed-inbounds">
            {policy.managedInbounds.map((inbound) => (
              <div className="lb-managed-inbound" key={inbound.tag}>
                <Tag color="blue">tcp</Tag>
                <Typography.Text className="lb-inbound-address" code>
                  {inbound.listen || '0.0.0.0'}:{inbound.port}
                </Typography.Text>
              </div>
            ))}
          </div>
        ) : (
          <Typography.Text type="secondary">-</Typography.Text>
        ),
    },
    {
      title: '活动连接',
      dataIndex: 'activeConnections',
      key: 'connections',
      width: '8%',
      align: 'center',
      render: (count: number) => count,
    },
    {
      title: '下发状态',
      key: 'apply',
      width: '10%',
      align: 'center',
      render: (_, policy) => (
        <Space direction="vertical" align="center" size={2}>
          <Tag color={applyColor(policy.status)}>{applyLabel(policy.status)}</Tag>
          {(policy.lastError || policy.applyError) && (
            <Tooltip title={policy.lastError || policy.applyError}>
              <Typography.Text type="danger" ellipsis style={{ maxWidth: 220, display: 'block' }}>
                {policy.lastError || policy.applyError}
              </Typography.Text>
            </Tooltip>
          )}
        </Space>
      ),
    },
    {
      title: '操作',
      key: 'actions',
      width: '12%',
      align: 'center',
      render: (_, policy) =>
        policy.remote ? null : (
          <Button
            size="small"
            icon={<UndoOutlined aria-hidden />}
            loading={
              resetMutation.isPending && resetTarget === `${policy.nodeKey}|${policy.clientId}`
            }
            onClick={() => {
              const key = `${policy.nodeKey}|${policy.clientId}`;
              setResetTarget(key);
              resetMutation.mutate({ nodeKey: policy.nodeKey, clientId: policy.clientId });
            }}
          >
            {'重置用量'}
          </Button>
        ),
    },
  ];

  const syncLabel = (() => {
    const status = data?.sync?.status;
    if (status === 'ok') return '正常';
    if (status === 'error') return '错误';
    return '空闲';
  })();

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout
        className={['nodes-page', isDark && 'is-dark', isUltra && 'is-ultra']
          .filter(Boolean)
          .join(' ')}
      >
        <AppSidebar />
        <Layout className="content-shell">
          <Layout.Content id="content-layout" className="content-area">
            <div className="load-balancing-page">
              <div className="lb-header">
                <div>
                  <Typography.Title level={3} style={{ marginBottom: 4 }}>
                    {'负载均衡'}
                  </Typography.Title>
                </div>
                <Space wrap>
                  <Button
                    icon={<SyncOutlined aria-hidden />}
                    loading={syncMutation.isPending}
                    onClick={() => syncMutation.mutate()}
                  >
                    {'立即同步'}
                  </Button>
                  <Button
                    icon={<ReloadOutlined aria-hidden />}
                    loading={query.isFetching}
                    onClick={() => query.refetch()}
                  >
                    {'刷新'}
                  </Button>
                </Space>
              </div>

              {query.error && (
                <Alert type="error" showIcon message={(query.error as Error).message} />
              )}

              <Card className="lb-summary-card" size="small">
                <Row gutter={[16, 12]}>
                  <Col xs={12} sm={8} md={4}>
                    <Statistic title="客户端" value={statistics.clients} />
                  </Col>
                  <Col xs={12} sm={8} md={4}>
                    <Statistic
                      title="在线节点"
                      value={statistics.onlineNodes}
                      prefix={<span className="lb-stat-dot lb-stat-dot-online" />}
                    />
                  </Col>
                  <Col xs={12} sm={8} md={4}>
                    <Statistic
                      title="正常"
                      value={statistics.normal}
                      prefix={<span className="lb-stat-dot lb-stat-dot-normal" />}
                    />
                  </Col>
                  <Col xs={12} sm={8} md={4}>
                    <Statistic
                      title="宽限"
                      value={statistics.grace}
                      prefix={<span className="lb-stat-dot lb-stat-dot-grace" />}
                    />
                  </Col>
                  <Col xs={12} sm={8} md={4}>
                    <Statistic
                      title="停用"
                      value={statistics.disabled}
                      prefix={<span className="lb-stat-dot lb-stat-dot-disabled" />}
                    />
                  </Col>
                  <Col xs={12} sm={8} md={4}>
                    <Statistic
                      title="未应用"
                      value={statistics.unapplied}
                      prefix={<span className="lb-stat-dot lb-stat-dot-pending" />}
                    />
                  </Col>
                </Row>
              </Card>

              <Tabs
                className="lb-tabs"
                activeKey={activeTab}
                onChange={setActiveTab}
                items={[
                  {
                    key: 'nodes',
                    label: '节点额度',
                    children: (
                      <Card
                        title={`节点额度与权重（${data?.timezone ?? ''}）`}
                        extra={
                          <Button type="primary" loading={saveMutation.isPending} onClick={onSave}>
                            {'保存配置'}
                          </Button>
                        }
                      >
                        <Table
                          className="lb-node-table"
                          rowKey="key"
                          size="small"
                          pagination={false}
                          loading={query.isLoading}
                          dataSource={data?.nodes ?? []}
                          tableLayout="fixed"
                          columns={nodeColumns}
                        />
                      </Card>
                    ),
                  },
                  {
                    key: 'policies',
                    label: '策略状态',
                    children: (
                      <Card
                        title="策略状态"
                        extra={
                          <Typography.Text type="secondary">
                            {'同步'}: {syncLabel}
                            {data?.sync?.lastSync ? ` · ${formatTime(data.sync.lastSync)}` : ''}
                          </Typography.Text>
                        }
                      >
                        <div className="lb-policy-filters">
                          <Select
                            allowClear
                            showSearch
                            optionFilterProp="label"
                            aria-label="筛选客户端"
                            placeholder="全部客户端"
                            value={clientFilter}
                            options={clientOptions}
                            onChange={setClientFilter}
                          />
                          <Select
                            allowClear
                            showSearch
                            optionFilterProp="label"
                            aria-label="筛选节点"
                            placeholder="全部节点"
                            value={nodeFilter}
                            options={(data?.nodes ?? []).map((node) => ({
                              value: node.key,
                              label: node.isLocal ? '本机' : node.name,
                            }))}
                            onChange={setNodeFilter}
                          />
                        </div>
                        <Table
                          className="lb-policy-table"
                          rowKey={(policy) =>
                            `${policy.masterGuid}|${policy.nodeKey}|${policy.clientId}`
                          }
                          size="small"
                          tableLayout="fixed"
                          pagination={false}
                          loading={query.isLoading}
                          dataSource={filteredPolicies}
                          columns={policyColumns}
                          locale={{ emptyText: <Empty description={'没有符合条件的策略。'} /> }}
                        />
                      </Card>
                    ),
                  },
                ]}
              />
            </div>
          </Layout.Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
}
