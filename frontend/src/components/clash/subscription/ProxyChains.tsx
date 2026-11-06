import { useEffect, useRef, useState } from 'react';
import { Alert, Button, Dropdown, Tag } from 'antd';
import { DeleteOutlined, ArrowDownOutlined, PlusOutlined } from '@ant-design/icons';
import Sortable from 'sortablejs';
import { templateDependencies, dependencyCycle } from '@/lib/clash/subscription/templateValidation';
import '@/components/clash/subscription/Templates.css';

interface Props {
  content: string;
  readonly: boolean;
  onChange: (content: string) => void;
  onValidityChange?: (valid: boolean) => void;
}
export function readProxyChain(content: string, exit: string) {
  const { nodes } = templateDependencies(content);
  const chain: string[] = [];
  const seen = new Set<string>();
  let cursor: string | undefined = exit;
  while (cursor !== undefined) {
    if (seen.has(cursor)) throw Error('链条存在循环依赖');
    seen.add(cursor);
    chain.unshift(cursor);
    cursor = nodes.find((n) => n.name === cursor)?.via;
  }
  return chain;
}
export function writeProxyChain(content: string, original: string[], chain: string[]) {
  const { doc, nodes } = templateDependencies(content);
  if (doc.errors.length) throw Error(doc.errors[0].message);
  if (new Set(nodes.map((n) => n.name)).size !== nodes.length) throw Error('代理或策略组名称重复');
  if (new Set(chain).size !== chain.length) throw Error('同一节点不能在链中重复出现');
  for (const [i, name] of chain.entries()) {
    const node = nodes.find((n) => n.name === name);
    if (!node && name !== 'DIRECT') throw Error('节点不存在：' + name);
    if (i > 0 && node?.kind !== 'proxy')
      throw Error('策略组和 DIRECT 只能作为入口，其后必须是具体代理');
  }
  for (const name of new Set([...original, ...chain])) {
    const node = nodes.find((n) => n.name === name);
    if (node?.kind !== 'proxy') continue;
    const index = chain.indexOf(name);
    if (index > 0) {
      doc.setIn(['proxies', node.index, 'dialer-proxy'], chain[index - 1]);
      node.dependencies = [chain[index - 1]];
    } else {
      doc.deleteIn(['proxies', node.index, 'dialer-proxy']);
      node.dependencies = [];
    }
  }
  const cycle = dependencyCycle(nodes);
  if (cycle) throw Error('循环依赖：' + cycle.join(' → '));
  return doc.toString();
}
export default function ProxyChains({ content, readonly, onChange, onValidityChange }: Props) {
  const { doc, nodes } = templateDependencies(content);
  const exits = nodes.filter(
    (n) =>
      n.kind === 'proxy' && n.via !== undefined && !nodes.some((other) => other.via === n.name),
  );
  const [pending, setPending] = useState<{ content: string; chain: string[]; error: string }>();
  const cycle = dependencyCycle(nodes);
  const sourceIssue =
    doc.errors[0]?.message ||
    (cycle
      ? '循环依赖：' + cycle.join(' → ')
      : exits.length > 1
        ? '当前 YAML 包含多条代理链，请先在 YAML 中整理为一条。'
        : '');
  const currentPending = pending?.content === content ? pending : undefined;
  const persistedChain =
    !sourceIssue && exits.length === 1 ? readProxyChain(content, exits[0].name) : [];
  const chain = currentPending?.chain ?? persistedChain;
  const issue = sourceIssue || currentPending?.error || '';
  const editable = !readonly && !sourceIssue;
  useEffect(() => {
    onValidityChange?.(!issue);
  }, [issue, onValidityChange]);
  function apply(next: string[]) {
    if (!editable) return;
    try {
      const updated = writeProxyChain(content, persistedChain, next);
      setPending({ content: updated, chain: next, error: '' });
      onChange(updated);
    } catch (e) {
      setPending({ content, chain: next, error: e instanceof Error ? e.message : String(e) });
    }
  }
  const list = useRef<HTMLDivElement>(null);
  const update = useRef<(from: number, to: number) => void>(() => {});
  useEffect(() => {
    update.current = (from, to) => {
      if (from === to || from < 0 || to < 0 || from >= chain.length || to >= chain.length) return;
      const next = [...chain];
      const [name] = next.splice(from, 1);
      next.splice(to, 0, name);
      apply(next);
    };
  });
  useEffect(() => {
    if (!list.current || !editable) return;
    let next: Element | null = null;
    let cancelled = false;
    const cancel = (event: KeyboardEvent) => {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      event.stopPropagation();
      cancelled = true;
      document.dispatchEvent(new PointerEvent('pointercancel', { bubbles: true }));
    };
    const options: Sortable.Options & { supportPointer: boolean } = {
      draggable: '[data-chain-row]',
      handle: '.template-sort-handle',
      forceFallback: true,
      supportPointer: true,
      fallbackClass: 'chain-drag-preview',
      ghostClass: 'chain-drop-position',
      scroll: true,
      bubbleScroll: true,
      onChoose({ item }) {
        next = item.nextElementSibling;
        cancelled = false;
        document.addEventListener('keydown', cancel, true);
      },
      onUnchoose() {
        document.removeEventListener('keydown', cancel, true);
      },
      onEnd({ item, from, oldDraggableIndex, newDraggableIndex }) {
        from.insertBefore(item, next);
        if (!cancelled && oldDraggableIndex !== undefined && newDraggableIndex !== undefined)
          update.current(oldDraggableIndex, newDraggableIndex);
      },
    };
    const sortable = Sortable.create(list.current, options);
    return () => {
      document.removeEventListener('keydown', cancel, true);
      sortable.destroy();
    };
  }, [editable]);
  function rows(chain: string[], editable: boolean) {
    return chain.map((name, index) => {
      const kind = nodes.find((n) => n.name === name)?.kind;
      return (
        <div key={name} data-chain-row>
          <div
            className={
              'proxy-chain-step ' +
              (index === 0 ? 'is-entry' : index === chain.length - 1 ? 'is-exit' : '')
            }
          >
            {editable && (
              <button
                type="button"
                className="template-sort-handle"
                title="拖动排序 · Esc 取消 · ↑ ↓"
                aria-label={'移动 ' + name}
                onKeyDown={(e) => {
                  const to = index + (e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0);
                  if (to !== index && to >= 0 && to < chain.length) {
                    e.preventDefault();
                    update.current(index, to);
                  }
                }}
              >
                ⠿
              </button>
            )}
            <Tag color={index === 0 ? 'green' : index === chain.length - 1 ? 'orange' : 'blue'}>
              {index === 0 ? '入口' : index === chain.length - 1 ? '出口' : index + 1}
            </Tag>
            <span className="proxy-chain-name">{name}</span>
            <Tag>
              {kind === 'group'
                ? '策略组'
                : name === 'DIRECT'
                  ? 'DIRECT'
                  : String(
                      doc.getIn([
                        'proxies',
                        nodes.find((n) => n.name === name)?.index ?? -1,
                        'type',
                      ]) ?? '代理',
                    ).toUpperCase()}
            </Tag>
            {editable && (
              <Button
                type="text"
                danger
                icon={<DeleteOutlined aria-hidden />}
                aria-label={'移除 ' + name}
                onClick={() => apply(chain.filter((n) => n !== name))}
              />
            )}
          </div>
          {index < chain.length - 1 && (
            <div className="proxy-chain-arrow">
              <ArrowDownOutlined aria-hidden />
            </div>
          )}
        </div>
      );
    });
  }
  return (
    <div className="template-entry-list">
      {issue && <Alert type="error" showIcon title={issue} />}
      <div ref={list} className="proxy-chain-list">
        {rows(chain, editable)}
      </div>
      {!readonly && (
        <Dropdown
          trigger={['click']}
          disabled={!editable}
          menu={{
            items: [...new Set([...nodes.map((n) => n.name), 'DIRECT'])]
              .filter((name) => !chain.includes(name))
              .map((name) => ({ key: name, label: name })),
            onClick: ({ key }) => apply([...chain, key]),
            style: { maxHeight: 320, overflowY: 'auto' },
          }}
        >
          <Button
            className="template-entry-add"
            icon={<PlusOutlined aria-hidden />}
            disabled={
              !editable ||
              [...nodes.map((n) => n.name), 'DIRECT'].every((name) => chain.includes(name))
            }
          >
            添加链式节点
          </Button>
        </Dropdown>
      )}
    </div>
  );
}
