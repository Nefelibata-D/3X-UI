import { replaceSymbol, templateSymbols } from './templateSymbols';
import ProxyChains from './ProxyChains';
import SortableTemplateRow, { SortableTemplateList } from './SortableTemplateRow';
import { useState, useId } from 'react';
import type { ReactNode } from 'react';
import { Alert, Button, Form, Input, Select, Space, Tag } from 'antd';
import { DeleteOutlined, EditOutlined, PlusOutlined } from '@ant-design/icons';
import { isMap, isScalar, isSeq, parseDocument } from 'yaml';

interface Props {
  entryActions?: (name: string) => ReactNode;
  entryClassName?: (name: string) => string;
  content: string;
  onChange: (value: string) => void;
  tab: 'providers' | 'routes' | 'dialer';
  groups: string[];
  rules: { id: number; name: string; code: string; filename: string; routing: boolean }[];
  readonly: boolean;
  onValidityChange?: (valid: boolean) => void;
}

function panelURL(rule: Props['rules'][number]) {
  return new URL(
    (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '') +
      '/rulesets/' +
      rule.code +
      '/' +
      encodeURIComponent(rule.filename),
    location.origin,
  ).href;
}
const intervalUnits = [
  { value: 1, label: '秒' },
  { value: 60, label: '分钟' },
  { value: 3600, label: '小时' },
  { value: 86400, label: '天' },
];
const routeRuleTypes = [
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
  'RULE-SET',
  'SUB-RULE',
  'AND',
  'OR',
  'NOT',
  'MATCH',
];
const routeRuleLabels: Record<string, string> = {
  DOMAIN: '匹配完整域名',
  'DOMAIN-SUFFIX': '匹配域名后缀',
  'DOMAIN-KEYWORD': '匹配域名关键字',
  'DOMAIN-REGEX': '正则匹配域名',
  'DOMAIN-WILDCARD': '通配符匹配域名',
  GEOSITE: '匹配 GeoSite',
  GEOIP: '匹配 GeoIP',
  'SRC-GEOIP': '匹配源 GeoIP',
  'IP-ASN': '匹配 IP ASN',
  'SRC-IP-ASN': '匹配源 IP ASN',
  'IP-CIDR': '匹配 IPv4 网段',
  'IP-CIDR6': '匹配 IPv6 网段',
  'SRC-IP-CIDR': '匹配源 IP 网段',
  'IP-SUFFIX': '匹配 IP 后缀',
  'SRC-IP-SUFFIX': '匹配源 IP 后缀',
  'SRC-PORT': '匹配源端口',
  'DST-PORT': '匹配目标端口',
  'IN-PORT': '匹配入站端口',
  DSCP: '匹配 DSCP',
  'PROCESS-NAME': '匹配进程名',
  'PROCESS-PATH': '匹配进程路径',
  'PROCESS-NAME-REGEX': '正则匹配进程名',
  'PROCESS-PATH-REGEX': '正则匹配进程路径',
  'PROCESS-NAME-WILDCARD': '通配符匹配进程名',
  'PROCESS-PATH-WILDCARD': '通配符匹配进程路径',
  NETWORK: '匹配网络类型',
  UID: '匹配用户 ID',
  'IN-TYPE': '匹配入站类型',
  'IN-USER': '匹配入站用户',
  'IN-NAME': '匹配入站名称',
  'REMATCH-NAME': '匹配重映射名称',
  'RULE-SET': '引用已声明规则集',
  'SUB-RULE': '引用子规则',
  AND: '同时匹配全部条件',
  OR: '匹配任一条件',
  NOT: '反向匹配条件',
  MATCH: '匹配剩余请求',
};
const policyNames: Record<string, string> = {
  DIRECT: '直接连接',
  REJECT: '拦截请求',
  'REJECT-DROP': '丢弃请求',
  PASS: '跳过此项',
};
const routeTargets = Object.keys(policyNames);
function displayInterval(seconds: number) {
  const unit =
    intervalUnits
      .slice()
      .reverse()
      .find(
        ({ value }) =>
          seconds >= value && seconds % value === 0 && !(value === 86400 && seconds === 86400),
      ) ?? intervalUnits[0];
  return { amount: String(seconds / unit.value), unit };
}
function behaviorLabel(behavior: string) {
  switch (behavior.toLowerCase()) {
    case 'classical':
      return 'Classical';
    case 'domain':
      return 'Domain';
    case 'ipcidr':
      return 'IPCIDR';
    default:
      return behavior;
  }
}
type Entry = string | number;
function parseRouteRule(rule: string) {
  const parts = rule.split(',').map((part) => part.trim());
  const type = (parts[0] || 'RULE-SET').toUpperCase();
  if (parts.length < 2) return { type, content: '', target: 'DIRECT', params: [] as string[] };
  if (type === 'MATCH') {
    return { type, content: '', target: parts[1], params: [] };
  }
  if (
    [
      'AND',
      'OR',
      'NOT',
      'SUB-RULE',
      'DOMAIN-REGEX',
      'PROCESS-NAME-REGEX',
      'PROCESS-PATH-REGEX',
    ].includes(type)
  ) {
    return {
      type,
      content: parts.slice(1, -1).join(','),
      target: parts.at(-1) || 'DIRECT',
      params: [],
    };
  }
  return {
    type,
    content: parts[1],
    target: parts[2] || 'DIRECT',
    params: parts.slice(3),
  };
}
export default function TemplateInsert(props: Props) {
  const { tab, content, onChange, readonly } = props;
  const [editing, setEditing] = useState<{ entry?: Entry; original: string }>();
  const doc = parseDocument(content);
  const collection = tab === 'routes' ? 'rules' : 'rule-providers';
  const items = doc.get(collection);
  const rows = isMap(items)
    ? items.items.map((p) => ({ entry: String(p.key), label: String(p.key) }))
    : isSeq(items)
      ? items.items.map((v, i) => ({ entry: i, label: String(v) }))
      : [];
  if (tab === 'dialer')
    return (
      <ProxyChains
        onValidityChange={props.onValidityChange}
        content={content}
        readonly={readonly}
        onChange={onChange}
      />
    );
  const label = tab === 'routes' ? '路由' : '规则集';
  const stale = editing && editing.original !== content;
  const editor = editing && !readonly && !stale && (
    <EntryForm {...props} entry={editing.entry} finish={() => setEditing(undefined)} />
  );
  return (
    <SortableTemplateList
      content={content}
      collection={collection}
      enabled={!readonly && !editing && !doc.errors.length}
      onChange={onChange}
    >
      {rows.map(({ entry, label: title }) => {
        const route = tab === 'routes' ? parseRouteRule(title) : undefined;
        if (route && editing?.entry === entry && editor) return <div key={entry}>{editor}</div>;
        const node = doc.getIn([collection, entry]);
        const behavior = isMap(node) ? String(node.get('behavior') ?? '') : '';
        const interval = isMap(node) ? node.get('interval') : undefined;
        const seconds = Number(interval);
        const { amount, unit } = displayInterval(seconds);
        const frequency =
          interval == null ? '' : seconds === 0 ? '不定时更新' : amount + unit.label + '更新';
        return (
          <div
            key={entry}
            data-sort-item
            className={[route ? 'template-route-card' : '', props.entryClassName?.(title)]
              .filter(Boolean)
              .join(' ')}
          >
            <SortableTemplateRow
              showHandle={!readonly}
              enabled={!readonly && !editing && !doc.errors.length}
            >
              <span className="template-entry-title" title={title}>
                {route ? route.content || routeRuleLabels[route.type] || route.type : title}
              </span>
              {route && (
                <div className="template-route-tags">
                  <Tag color="blue" title="匹配类型">
                    {route.type}
                  </Tag>
                  <Tag color="purple" title="路由目标">
                    {route.target}
                  </Tag>
                </div>
              )}
              {behavior && <Tag color="blue">{behaviorLabel(behavior)}</Tag>}
              {frequency && <Tag>{frequency}</Tag>}
              {!readonly && (
                <Space size={4}>
                  <Button
                    type="text"
                    title="编辑"
                    className={editing?.entry === entry ? 'template-edit-active' : undefined}
                    aria-pressed={editing?.entry === entry}
                    aria-label={'编辑 ' + title}
                    icon={<EditOutlined aria-hidden />}
                    disabled={!!editing || !!doc.errors.length}
                    onClick={() => setEditing({ entry, original: content })}
                  />
                  <Button
                    type="text"
                    danger
                    title="移除"
                    aria-label={'移除 ' + title}
                    icon={<DeleteOutlined aria-hidden />}
                    disabled={!!editing || !!doc.errors.length}
                    onClick={() => {
                      doc.deleteIn([collection, entry]);
                      onChange(doc.toString());
                    }}
                  />
                </Space>
              )}
              {props.entryActions?.(title)}
            </SortableTemplateRow>
            {editing?.entry === entry && editor}
          </div>
        );
      })}
      {stale && (
        <Alert
          type="warning"
          title="基础 YAML 已变更，请重新选择条目。"
          action={<Button onClick={() => setEditing(undefined)}>关闭</Button>}
        />
      )}
      {editing && editing.entry === undefined && editor}
      {!readonly && !editing && (
        <Button
          className="template-entry-add"
          icon={<PlusOutlined aria-hidden />}
          disabled={!!doc.errors.length}
          onClick={() => setEditing({ original: content })}
        >
          添加{label}
        </Button>
      )}
    </SortableTemplateList>
  );
}

function EntryForm({
  content,
  onChange,
  tab,
  groups,
  rules,
  readonly,
  entry,
  finish,
}: Props & { entry?: Entry; finish?: () => void }) {
  const doc = parseDocument(content);
  const existing = typeof entry === 'string' ? doc.getIn(['rule-providers', entry]) : undefined;
  const field = (key: string, fallback: string) =>
    isMap(existing) ? String(existing.get(key) ?? fallback) : fallback;
  const oldRule = typeof entry === 'number' ? String(doc.getIn(['rules', entry])) : '';
  const parsedRule = parseRouteRule(oldRule);
  const uid = useId();
  const originalPanel = rules.find((r) => panelURL(r) === field('url', ''));
  const [source, setSource] = useState(originalPanel ? 'panel' : field('type', 'panel'));
  const [name, setName] = useState(typeof entry === 'string' ? entry : '');
  const [address, setAddress] = useState(field('url', ''));
  const [path, setPath] = useState(field('path', ''));
  const [behavior, setBehavior] = useState(field('behavior', 'classical'));
  const [format, setFormat] = useState(field('format', 'yaml'));
  const initialInterval = displayInterval(Number(field('interval', '86400')));
  const [amount, setAmount] = useState(initialInterval.amount);
  const [unit, setUnit] = useState(initialInterval.unit.value);
  const [panelId, setPanelId] = useState<number | undefined>(originalPanel?.id);
  const oldPayload = isMap(existing) ? existing.get('payload') : undefined;
  const [payload, setPayload] = useState(
    isSeq(oldPayload) ? oldPayload.items.map(String).join('\n') : '',
  );
  const [ruleType, setRuleType] = useState(parsedRule.type);
  const [ruleContent, setRuleContent] = useState(parsedRule.content);
  const [provider, setProvider] = useState<string | undefined>(
    parsedRule.type === 'RULE-SET' ? parsedRule.content : undefined,
  );
  const [target, setTarget] = useState(parsedRule.target);
  const [error, setError] = useState('');

  const providers = doc.get('rule-providers');
  const subRules = doc.get('sub-rules');
  const names = isMap(providers) ? providers.items.map((p) => String(p.key)) : [];
  const subRuleNames = isMap(subRules) ? subRules.items.map((p) => String(p.key)) : [];
  const opts = (values: string[]) => values.map((value) => ({ label: value, value }));
  const panel = rules.find((r) => r.id === panelId);
  const http = source === 'panel' || source === 'http';
  function insert() {
    try {
      if (doc.errors.length) throw doc.errors[0];
      if (tab === 'providers') {
        const key = name.trim();
        if (!key || /[,\r\n]/.test(key)) throw Error('请填写可用于 RULE-SET 的名称');
        if (entry !== key && doc.hasIn(['rule-providers', key])) throw Error('该名称已声明');
        const item: Record<string, unknown> = {
          type: http ? 'http' : source,
          behavior: source === 'panel' ? 'classical' : behavior,
        };
        if (http) {
          if (source === 'panel' && !panel) throw Error('请选择面板规则集');
          const url =
            source === 'panel' && panel
              ? new URL(
                  (window.X_UI_BASE_PATH || '/').replace(/\/+$/, '') +
                    '/rulesets/' +
                    panel.code +
                    '/' +
                    encodeURIComponent(panel.filename),
                  location.origin,
                )
              : new URL(address);
          if (!['http:', 'https:'].includes(url.protocol)) throw Error('只支持 HTTP 或 HTTPS 地址');
          item.url = url.href;
          const seconds = Number(amount) * unit;
          if (!/^\d+$/.test(amount) || !Number.isSafeInteger(seconds))
            throw Error('请填写可精确表示的非负整数更新间隔');
          item.interval = seconds;
        }
        if (source === 'inline') {
          const lines = payload
            .split(/\r?\n/)
            .map((v) => v.trim())
            .filter(Boolean);
          if (!lines.length) throw Error('请填写内联规则');
          item.payload = lines;
        } else {
          const file = path.trim();
          if (
            (source === 'file' && !file) ||
            (file &&
              (/[:\\]/.test(file) ||
                Array.from(file).some((c) => c.charCodeAt(0) < 32) ||
                file
                  .replace(/^\.\//, '')
                  .split('/')
                  .some((v) => !v || v === '.' || v === '..')))
          )
            throw Error('请使用 Mihomo 工作目录下的相对路径，如 rules/media.yaml');
          if (file) item.path = file;
          item.format = source === 'panel' ? 'yaml' : format;
          if (item.format === 'mrs' && behavior === 'classical')
            throw Error('MRS 仅支持 domain 或 ipcidr');
        }
        if (isMap(existing)) {
          for (const attr of [
            'type',
            'behavior',
            'url',
            'interval',
            'path',
            'format',
            ...(source === 'inline' || field('type', '') === 'inline' ? ['payload'] : []),
          ]) {
            if (!(attr in item)) existing.delete(attr);
          }
          if (typeof entry === 'string' && entry !== key && isMap(providers)) {
            const pair = providers.items.find((p) => String(p.key) === entry);
            if (!pair || !isScalar(pair.key)) throw Error('规则集名称无法修改');
            replaceSymbol(templateSymbols(doc), 'ruleset', entry, key);
          }
          for (const [attr, val] of Object.entries(item)) existing.set(attr, doc.createNode(val));
        } else doc.setIn(['rule-providers', key], doc.createNode(item));
        setProvider(key);
      } else if (tab === 'routes') {
        const availableTargets = [...routeTargets, ...groups];
        if (!availableTargets.includes(target)) throw Error('目标必须是内置策略或 Proxy Group');
        if (!routeRuleTypes.includes(ruleType)) throw Error('请选择 Mihomo 支持的规则类型');
        if (ruleType === 'RULE-SET' && !names.includes(provider ?? ''))
          throw Error('请选择已声明的规则集');
        if (ruleType === 'SUB-RULE' && !subRuleNames.includes(ruleContent))
          throw Error('请选择已声明的子规则');
        if (ruleType !== 'MATCH' && ruleType !== 'RULE-SET' && !ruleContent.trim())
          throw Error('请填写规则内容');
        const content =
          ruleType === 'RULE-SET' ? provider : ruleType === 'MATCH' ? '' : ruleContent.trim();
        const params = ruleType === parsedRule.type ? parsedRule.params : [];
        const rule = [ruleType, content, target, ...params].filter((part) => part !== '').join(',');
        if (/[\r\n]/.test(rule)) throw Error('规则内容不能包含换行');
        if (!doc.has('rules')) doc.set('rules', doc.createNode([]));
        const seq = doc.get('rules');
        if (!isSeq(seq)) throw Error('rules 必须是列表');
        const fallback = seq.items.findIndex((v) => String(v).startsWith('MATCH,'));
        if (typeof entry === 'number') seq.set(entry, doc.createNode(rule));
        else seq.items.splice(fallback < 0 ? seq.items.length : fallback, 0, doc.createNode(rule));
      }
      onChange(doc.toString());
      finish?.();
      setError('');
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }
  return (
    <Form
      layout="horizontal"
      className={finish ? 'template-entry-form template-entry-edit' : 'template-entry-form'}
      size="middle"
      colon={false}
      disabled={readonly}
    >
      {tab === 'providers' && (
        <>
          <Form.Item htmlFor={uid + '-1'} label="来源">
            <Select
              id={uid + '-1'}
              value={source}
              onChange={setSource}
              options={[
                { value: 'panel', label: '面板规则集' },
                { value: 'http', label: '外部 HTTP / HTTPS' },
                { value: 'file', label: '客户端文件' },
                { value: 'inline', label: '内联规则（inline）' },
              ]}
            />
          </Form.Item>
          {source === 'panel' && (
            <Form.Item htmlFor={uid + '-2'} label="规则文件">
              <Select
                id={uid + '-2'}
                value={panelId}
                options={rules
                  .filter((r) => r.routing)
                  .map((r) => ({ value: r.id, label: r.name }))}
                onChange={(v) => {
                  setPanelId(v);
                  if (entry === undefined) setName(rules.find((r) => r.id === v)?.name ?? '');
                }}
              />
            </Form.Item>
          )}
          <Form.Item htmlFor={uid + '-3'} label="规则集名称">
            <Input id={uid + '-3'} value={name} onChange={(e) => setName(e.target.value)} />
          </Form.Item>
          {source === 'http' && (
            <Form.Item htmlFor={uid + '-4'} label="文件地址">
              <Input
                id={uid + '-4'}
                value={address}
                onChange={(e) => setAddress(e.target.value)}
                placeholder="https://…"
              />
            </Form.Item>
          )}
          {source !== 'panel' && (
            <Form.Item htmlFor={uid + '-5'} label="匹配方式">
              <Select
                id={uid + '-5'}
                value={behavior}
                options={opts(['classical', 'domain', 'ipcidr'])}
                onChange={setBehavior}
              />
            </Form.Item>
          )}
          {source !== 'inline' && source !== 'panel' && (
            <Form.Item htmlFor={uid + '-6'} label="文件格式">
              <Select
                id={uid + '-6'}
                value={format}
                options={opts(['yaml', 'text', 'mrs'])}
                onChange={setFormat}
              />
            </Form.Item>
          )}
          {source === 'inline' ? (
            <Form.Item htmlFor={uid + '-7'} label="规则内容">
              <Input.TextArea
                id={uid + '-7'}
                value={payload}
                onChange={(e) => setPayload(e.target.value)}
              />
            </Form.Item>
          ) : (
            <Form.Item htmlFor={uid + '-8'} label={http ? '缓存路径' : '文件路径'}>
              <Input
                id={uid + '-8'}
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder={http ? '可选，默认自动生成' : 'rules/media.yaml'}
              />
            </Form.Item>
          )}
          {http && (
            <Form.Item htmlFor={uid + '-9'} label="更新间隔">
              <Space.Compact block className="template-update-interval">
                <Input
                  id={uid + '-9'}
                  inputMode="numeric"
                  value={amount}
                  onChange={(e) => {
                    if (/^\d*$/.test(e.target.value)) setAmount(e.target.value);
                  }}
                />
                <Select
                  aria-label="更新间隔单位"
                  value={unit}
                  options={intervalUnits}
                  onChange={setUnit}
                />
              </Space.Compact>
            </Form.Item>
          )}
        </>
      )}
      {tab === 'routes' && (
        <>
          <Form.Item htmlFor={uid + '-10'} label="规则类型">
            <Select
              id={uid + '-10'}
              value={ruleType}
              onChange={(v) => setRuleType(v)}
              options={routeRuleTypes.map((value) => ({
                value,
                label: `${routeRuleLabels[value]} (${value})`,
              }))}
            />
          </Form.Item>
          {ruleType === 'RULE-SET' || ruleType === 'SUB-RULE' ? (
            <Form.Item htmlFor={uid + '-12'} label="规则内容">
              <Select
                id={uid + '-12'}
                value={ruleType === 'RULE-SET' ? provider : ruleContent || undefined}
                options={opts(ruleType === 'RULE-SET' ? names : subRuleNames)}
                onChange={(value) =>
                  ruleType === 'RULE-SET' ? setProvider(value) : setRuleContent(value)
                }
                placeholder={ruleType === 'RULE-SET' ? '选择已声明的规则集' : '选择已声明的子规则'}
              />
            </Form.Item>
          ) : ruleType !== 'MATCH' ? (
            <Form.Item htmlFor={uid + '-12'} label="规则内容">
              <Input.TextArea
                id={uid + '-12'}
                value={ruleContent}
                onChange={(e) => setRuleContent(e.target.value)}
                autoSize={{ minRows: 1, maxRows: 3 }}
                placeholder={
                  ruleType === 'AND' || ruleType === 'OR' || ruleType === 'NOT'
                    ? '例如 ((NETWORK,TCP),(DST-PORT,443))'
                    : '填写此类型的匹配内容'
                }
              />
            </Form.Item>
          ) : null}
          <Form.Item htmlFor={uid + '-13'} label="代理策略">
            <Select
              id={uid + '-13'}
              value={target}
              options={[
                ...routeTargets.map((value) => ({
                  value,
                  label: policyNames[value] + ' (' + value + ')',
                })),
                ...opts(groups),
              ]}
              onChange={setTarget}
            />
          </Form.Item>
        </>
      )}
      {error && <Alert type="error" title={error} />}
      <div className={finish ? 'template-entry-actions' : undefined}>
        <Space>
          {finish && <Button onClick={finish}>取消</Button>}
          <Button type="primary" onClick={insert}>
            确定
          </Button>
        </Space>
      </div>
    </Form>
  );
}
