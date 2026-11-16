import {
  createContext,
  useContext,
  useEffect,
  useRef,
  useLayoutEffect,
  useCallback,
  type ReactNode,
} from 'react';
import Sortable from 'sortablejs';
import { isMap, isSeq, parseDocument } from 'yaml';

const MoveEntry = createContext<((from: number, to: number) => void) | null>(null);

interface Props {
  collection: 'rule-providers' | 'proxy-groups' | 'rules';
  content: string;
  enabled: boolean;
  onChange: (value: string) => void;
  indices?: number[];
  children: ReactNode;
}

export function SortableTemplateList({
  collection,
  content,
  enabled,
  onChange,
  children,
  indices,
}: Props) {
  const root = useRef<HTMLDivElement>(null);
  const move = useCallback(
    (from: number, to: number) => {
      const doc = parseDocument(content);
      const list = doc.get(collection);
      if (!enabled || doc.errors.length || (!isMap(list) && !isSeq(list))) return;
      const items: unknown[] = list.items;
      const ordered = indices ? indices.map((index) => items[index]) : items;
      if (
        !Number.isInteger(from) ||
        from < 0 ||
        to < 0 ||
        from >= ordered.length ||
        to >= ordered.length ||
        from === to
      )
        return;
      const [item] = ordered.splice(from, 1);
      ordered.splice(to, 0, item);
      indices?.forEach((index, i) => {
        items[index] = ordered[i];
      });
      onChange(doc.toString());
    },
    [collection, content, enabled, onChange, indices],
  );

  const latestMove = useRef(move);
  useLayoutEffect(() => {
    latestMove.current = move;
  }, [move]);

  useEffect(() => {
    if (!root.current || !enabled) return;
    let originalIndex = 0;
    const ownerDocument = root.current.ownerDocument;
    function cancelOnEscape(event: KeyboardEvent) {
      if (event.key !== 'Escape') return;
      event.preventDefault();
      event.stopPropagation();
      ownerDocument.dispatchEvent(new PointerEvent('pointercancel', { bubbles: true }));
    }
    const options: Sortable.Options & { supportPointer: boolean } = {
      draggable: '[data-sort-item]',
      handle: '.template-sort-handle',
      forceFallback: true,
      supportPointer: true,
      fallbackTolerance: 3,
      fallbackClass: 'template-sort-preview',
      ghostClass: 'template-sort-position',
      scroll:
        root.current.closest<HTMLElement>('.subscription-route-scroll, .ant-tabs-body-holder') ??
        true,
      bubbleScroll: false,
      scrollSensitivity: 48,
      onChoose({ oldIndex }) {
        originalIndex = oldIndex ?? 0;
        ownerDocument.addEventListener('keydown', cancelOnEscape, true);
      },
      onUnchoose() {
        ownerDocument.removeEventListener('keydown', cancelOnEscape, true);
      },
      onEnd({
        item,
        from,
        oldDraggableIndex,
        newDraggableIndex,
        originalEvent,
      }: Sortable.SortableEvent & { originalEvent?: Event }) {
        const siblings = Array.from(from.children).filter((child) => child !== item);
        from.insertBefore(item, siblings[originalIndex] ?? null);
        if (originalEvent?.type === 'pointercancel') return;
        if (oldDraggableIndex !== undefined && newDraggableIndex !== undefined)
          latestMove.current(oldDraggableIndex, newDraggableIndex);
      },
    };
    const sortable = Sortable.create(root.current, options);
    return () => {
      ownerDocument.removeEventListener('keydown', cancelOnEscape, true);
      sortable.destroy();
    };
  }, [enabled, collection]);

  return (
    <MoveEntry value={move}>
      <div role="group" aria-label="可排序条目" ref={root} className="template-entry-list">
        {children}
      </div>
    </MoveEntry>
  );
}

export default function SortableTemplateRow({
  showHandle,
  enabled,
  children,
}: {
  showHandle: boolean;
  enabled: boolean;
  children: ReactNode;
}) {
  const move = useContext(MoveEntry);
  if (!move) throw new Error('SortableTemplateRow requires SortableTemplateList');
  return (
    <div className="template-entry-row">
      {showHandle && (
        <button
          onKeyDown={(event) => {
            if (event.key !== 'ArrowUp' && event.key !== 'ArrowDown') return;
            event.preventDefault();
            const item = event.currentTarget.closest('[data-sort-item]');
            if (!item?.parentElement) return;
            const items = Array.from(
              item.parentElement.querySelectorAll(':scope > [data-sort-item]'),
            );
            const index = items.indexOf(item);
            move(index, index + (event.key === 'ArrowUp' ? -1 : 1));
          }}
          className="template-sort-handle"
          type="button"
          disabled={!enabled}
          aria-label="拖动排序，Esc 取消，也可用上下方向键"
          title="拖动排序 · Esc 取消 · ↑ ↓"
        >
          ⋮
        </button>
      )}
      {children}
    </div>
  );
}
