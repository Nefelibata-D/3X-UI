import { useMemo, useState } from 'react';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import {
  Alert,
  Button,
  InputNumber,
  Select,
  Tooltip,
  Space,
  Spin,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import type { ColumnsType } from 'antd/es/table';
import { QuestionCircleOutlined } from '@ant-design/icons';

import './ClientLoadBalancingTab.css';
import { SizeFormatter } from '@/utils';
import { keys } from '@/api/queryKeys';
import { useClashClientRatioQuery } from '@/api/queries/useClashClientRatioQuery';
import { bytesToGB, gbToBytes, normalizePreview, saveClashClientRatio } from '@/lib/clash/api';
import type { ClientNodeView, ClientRatioView } from '@/schemas/clash';

interface ClientLoadBalancingTabProps {
  clientId?: number;
  email?: string;
  mode: 'add' | 'edit';
}

type RatioMode = 'inherit' | 'custom';


export default function ClientLoadBalancingTab({ clientId, mode }: ClientLoadBalancingTabProps) {
  const queryClient = useQueryClient();
  const query = useClashClientRatioQuery(mode === 'edit' ? clientId : undefined);
  const [edit, setEdit] = useState<{
    mode: RatioMode;
    weights: Record<string, number>;
  } | null>(null);
  const [limitEdits, setLimitEdits] = useState<Record<string, number> | null>(null);
  const limits = { ...Object.fromEntries((query.data?.nodes ?? []).map((n) => [n.nodeKey, bytesToGB(n.budgetBytes)])), ...limitEdits };
  const limitBytes = (node: ClientNodeView) => limitEdits?.[node.nodeKey] !== undefined ? gbToBytes(limitEdits[node.nodeKey]) : node.budgetBytes;
  const derivedMode: RatioMode = query.data?.mode === 'custom' ? 'custom' : 'inherit';
  const derivedWeights = useMemo(() => {
    const next: Record<string, number> = {};
    for (const node of query.data?.nodes ?? []) next[node.nodeKey] = derivedMode === "inherit" ? node.defaultWeight : node.overrideWeight;
    return next;
  }, [query.data, derivedMode]);
  const ratioMode = edit?.mode ?? derivedMode;
  const weights = edit?.weights ?? derivedWeights;

  const preview = useMemo(() => {
    if (!query.data) return {} as Record<string, number>;
    const entries = query.data.nodes
      .filter((node) => node.selectable)
      .map((node) => ({
        key: node.nodeKey,
        weight: ratioMode === 'inherit' ? node.defaultWeight : Number(weights[node.nodeKey]) || 0,
      }))
      .filter((entry) => entry.weight > 0);
    const out: Record<string, number> = {};
    for (const row of normalizePreview(entries)) out[row.key] = row.ratio;
    return out;
  }, [query.data, ratioMode, weights]);

  const mutation = useMutation({
    mutationFn: (vars: { mode: RatioMode; weights: Record<string, number>; limits: Record<string, number>; resetMode: "node" | "manual" }) =>
      saveClashClientRatio(clientId as number, vars.mode, vars.weights, vars.limits, vars.resetMode),
    onSuccess: (view: ClientRatioView) => {
      if (clientId) queryClient.setQueryData(keys.clash.client(clientId), view);
      queryClient.invalidateQueries({ queryKey: keys.clash.overview() });
      setEdit(null);
      setLimitEdits(null);
      message.success("限额与权重已保存。");
    },
    onError: (err: Error) => message.error(err.message),
  });

  if (mode === 'add' || !clientId) {
    return (
      <Alert
        type="info"
        showIcon
        message={"请先创建客户端，再设置负载均衡比例。"}
      />
    );
  }
  if (query.isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 24 }}>
        <Spin />
      </div>
    );
  }
  if (query.error) {
    return (
      <Alert
        type="error"
        showIcon
        message={"加载客户端比例失败。"}
        description={(query.error as Error).message}
      />
    );
  }
  if (!query.data) return null;
  if (query.data.nodes.length === 0) {
    return <Alert type="warning" showIcon message={"该客户端尚无获准出口，请先绑定入站。"} />;
  }

  const onSwitchMode = (next: RatioMode) => {
    const updated = { ...weights };
    setEdit({ mode: next, weights: updated });
  };

  const onSave = () => {
    const totalLimit = query.data.totalBytes;
    const allocatedTotal = query.data.nodes.reduce((sum, node) => sum + limitBytes(node), 0);
    if (totalLimit > 0) {
      if (query.data.nodes.some((node) => limitBytes(node) <= 0)) {
        message.error("客户端总流量有限时，每个节点额度也必须有限");
        return;
      }
      if (allocatedTotal > totalLimit) {
        message.error("节点流量额度合计不能超过客户端总流量上限");
        return;
      }
    }
    for (const node of query.data.nodes) {
      if (node.capacityBytes > 0 && (limitBytes(node) <= 0 || node.allocatedBytes - node.budgetBytes + limitBytes(node) > node.capacityBytes)) {
        message.error(`${node.name} 需要明确额度，且分配后不能超过节点总额度`); return;
      }
    }
    if (ratioMode === 'custom') {
      const sum = query.data.nodes
        .filter((node) => node.selectable)
        .reduce((acc, node) => acc + Math.max(0, Number(weights[node.nodeKey]) || 0), 0);
      if (sum <= 0) {
        message.error("自定义权重在可选出口内必须至少有一个正值。");
        return;
      }
    }
    const payload: Record<string, number> = {};
    for (const node of query.data.nodes) {
      const value = Math.max(0, Math.round(Number(weights[node.nodeKey]) || 0));
      if (value > 0) payload[node.nodeKey] = value;
    }
    mutation.mutate({ mode: ratioMode, resetMode: query.data.resetMode, weights: payload, limits: Object.fromEntries(query.data.nodes.map((n) => [n.nodeKey, limitBytes(n)])) });
  };

  const columns: ColumnsType<ClientNodeView> = [
    {
      title: "出口",
      key: 'outlet',
      width: 120,
      render: (_, node) => (
        <Space>
          <Typography.Text strong>{node.nodeKey === 'local' ? '本机' : node.name}</Typography.Text>
          {!node.selectable && <Tag>{'已禁用'}</Tag>}
        </Space>
      ),
    },
    {
      title: <Space size={4}>流量额度（GB）<Tooltip title="节点总额度有限时必须填写正数；只有无限节点可填 0，表示不限额。"><QuestionCircleOutlined aria-label="限额说明" /></Tooltip></Space>,
      key: 'budget',
      width: 230,
      render: (_, node) => {
        const clientTotalFinite = (query.data?.totalBytes ?? 0) > 0;
        const nodeLimitInvalid = (node.capacityBytes > 0 || clientTotalFinite) && limitBytes(node) <= 0;
        const nodeOverLimit = node.capacityBytes > 0 && node.allocatedBytes - node.budgetBytes + limitBytes(node) > node.capacityBytes;
        const totalLimit = query.data?.totalBytes ?? 0;
        const totalOverLimit = totalLimit > 0 && query.data!.nodes.reduce((sum, row) => sum + limitBytes(row), 0) > totalLimit;
        return (
          <div className="client-lb-budget-cell">
            <Space className="client-lb-budget" size={8}>
              <InputNumber min={0} precision={2} style={{ width: 100 }} aria-label={`${node.name} 月限额`}
                status={nodeLimitInvalid || nodeOverLimit || totalOverLimit ? 'error' : undefined}
                value={limits[node.nodeKey] ?? 0}
                onChange={(value) => setLimitEdits((prev) => ({ ...prev, [node.nodeKey]: Number(value) || 0 }))}
              />
              <Typography.Text type={nodeOverLimit ? 'danger' : 'secondary'} className="client-lb-budget-available">
                / {node.capacityBytes === 0 ? '∞' : SizeFormatter.sizeFormat(node.capacityBytes - node.allocatedBytes + node.budgetBytes - limitBytes(node))}
              </Typography.Text>
            </Space>
            {nodeLimitInvalid && <Typography.Text type="danger" className="client-lb-budget-error">总流量或节点总额有限时，节点额度必须为正数</Typography.Text>}
            {!nodeLimitInvalid && nodeOverLimit && <Typography.Text type="danger" className="client-lb-budget-error">超出节点可分配上限</Typography.Text>}
            {!nodeLimitInvalid && !nodeOverLimit && totalOverLimit && <Typography.Text type="danger" className="client-lb-budget-error">节点额度合计超过客户端总流量上限</Typography.Text>}
          </div>
        );
      },
    },
    {
      title: "权重",
      key: 'weight',
      width: 120,
      render: (_, node) => (
        <InputNumber
          min={0}
          step={1}
          precision={0}
          style={{ width: 56 }}
          aria-label={`${node.name} ${"权重"}`}
          disabled={ratioMode === 'inherit' || !node.selectable}
          value={weights[node.nodeKey] ?? 0}
          onChange={(value) => {
            setEdit({
              mode: ratioMode,
              weights: { ...weights, [node.nodeKey]: Number(value) || 0 },
            });
          }}
        />
      ),
    },
    {
      title: "节点权重比例",
      key: 'effective',
      width: 105,
      render: (_, node) =>
        node.selectable ? `${((preview[node.nodeKey] || 0) * 100).toFixed(1)}%` : '-',
    },
  ];

  return (
    <div className="client-lb-tab">
      <Typography.Paragraph type="secondary" style={{ marginBottom: 0 }}>
        {"额度从各节点独立扣减，权重默认继承。客户端基本页的总流量上限另行生效。"}
      </Typography.Paragraph>

      <Space wrap>
        <Typography.Text>{"分配方式"}</Typography.Text>
        <Select<RatioMode>
          value={ratioMode}
          onChange={onSwitchMode}
          style={{ width: 170 }}
          aria-label="分配方式"
          options={[{ value: 'inherit', label: '继承默认权重' }, { value: 'custom', label: '自定义权重' }]}
        />
      </Space>

      <Table
        scroll={{ x: ratioMode === "custom" ? 650 : 540 }}
        tableLayout="fixed"
        rowKey="nodeKey"
        size="small"
        pagination={false}
        dataSource={query.data.nodes}
        columns={columns.filter((column) => ratioMode === "custom" || column.key !== "weight").map((column) => ({ ...column, align: "center" as const }))}
      />

      <div className="client-lb-actions">
        <Button type="primary" loading={mutation.isPending} onClick={onSave}>
          {"保存限额与权重"}
        </Button>
      </div>
    </div>
  );
}
