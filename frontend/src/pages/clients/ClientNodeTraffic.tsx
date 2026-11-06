import ClientTrafficCell from '@/components/clients/ClientTrafficCell';
import { useClashClientRatioQuery } from '@/api/queries/useClashClientRatioQuery';
import './ClientNodeTraffic.css';

export default function ClientNodeTraffic({ clientId }: {
  clientId?: number;
}) {
  const query = useClashClientRatioQuery(clientId);
  if (!clientId) return <span>客户端标识缺失</span>;
  if (query.isLoading) return <span>加载中…</span>;
  if (query.error) return <span title={query.error.message}>节点流量读取失败</span>;
  if (!query.data?.nodes.length) return <span>未关联节点</span>;

  return (
    <div className="client-node-traffic">
      {query.data.nodes.map((node) => {
        const name = node.nodeKey === 'local' ? '本机' : node.name;
        return (
          <div className="client-node-traffic-row" key={node.nodeKey} title={node.applyError || undefined}>
            <span className="client-node-traffic-name" title={name}>{name}</span>
            {node.capacityBytes > 0 && node.budgetBytes === 0 ? <span>待分配额度</span> : !node.usageKnown ? <span>待回报</span> : (
              <ClientTrafficCell up={node.usageUp} down={node.usageDown} total={node.budgetBytes}
                enabled={node.state !== 'disabled'} compact />
            )}
            {node.applyError && <span className="client-node-traffic-error">回报异常</span>}
          </div>
        );
      })}
    </div>
  );
}
