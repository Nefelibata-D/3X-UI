import * as monaco from 'monaco-editor';
import { proxyTypes } from '@/lib/clash/subscription/templateSchema';
import { isMap, isSeq, isScalar, parseDocument, stringify } from 'yaml';
import {
  builtins,
  groupTypes,
  replaceSymbol,
  templateSymbols,
  type Slot,
  type SymbolKind,
} from '@/lib/clash/subscription/templateSymbols';

const labels: Record<SymbolKind, string> = {
  proxy: '代理或策略组',
  ruleset: '规则集',
  provider: '代理集合',
  subrule: '子规则',
  groupType: '策略组类型',
  proxyType: '代理协议类型',
};

export function installTemplateReferences(
  instance: monaco.editor.IStandaloneCodeEditor,
  model: monaco.editor.ITextModel,
) {
  const decorations = instance.createDecorationsCollection();
  let ranges = new Map<Slot, monaco.Range>();
  let names = new Map<SymbolKind, Set<string>>();
  function refresh() {
    const source = model.getValue();
    const doc = parseDocument(source);
    const slots = templateSymbols(doc);
    names = new Map();
    for (const s of slots.filter((s) => s.declaration)) {
      if (!names.has(s.kind)) names.set(s.kind, new Set());
      names.get(s.kind)!.add(s.name);
    }
    ranges = new Map();
    for (const s of slots) {
      if (!s.node.range) continue;
      const [a, b] = s.node.range;
      const raw = source.slice(a, b);
      const quoted = s.node.type === 'QUOTE_DOUBLE' || s.node.type === 'QUOTE_SINGLE';
      if ((quoted ? raw.slice(1, -1) : raw) !== String(s.node.value ?? '')) continue;
      const base = a + Number(quoted);
      ranges.set(
        s,
        monaco.Range.fromPositions(
          model.getPositionAt(base + s.start),
          model.getPositionAt(base + s.end),
        ),
      );
    }
    const ruleColors: monaco.editor.IModelDeltaDecoration[] = [];
    const paint = (start: number, end: number, kind: string) => {
      if (end <= start) return;
      ruleColors.push({
        range: monaco.Range.fromPositions(model.getPositionAt(start), model.getPositionAt(end)),
        options: { inlineClassName: `yaml-rule-${kind}` },
      });
    };
    const colorRules = (list: unknown, hasTarget: boolean) => {
      if (!isSeq(list)) return;
      for (const node of list.items) {
        if (!isScalar(node) || typeof node.value !== 'string' || !node.range) continue;
        const quoted = node.type === 'QUOTE_DOUBLE' || node.type === 'QUOTE_SINGLE';
        const start = node.range[0] + Number(quoted);
        const end = node.range[1] - Number(quoted);
        const text = source.slice(start, end);
        const comma = text.indexOf(',');
        if (comma < 0) continue;
        const target = hasTarget
          ? [...ranges].find(
              ([slot]) =>
                slot.node === node &&
                !slot.declaration &&
                (slot.kind === 'proxy' || slot.kind === 'subrule'),
            )
          : undefined;
        const targetStart = target ? model.getOffsetAt(target[1].getStartPosition()) : end;
        paint(start, start + comma, 'type');
        paint(start + comma + 1, target ? targetStart - 1 : end, 'match');
        if (target) paint(targetStart, model.getOffsetAt(target[1].getEndPosition()), 'policy');
      }
    };
    colorRules(doc.get('rules'), true);
    const subrules = doc.get('sub-rules');
    if (isMap(subrules)) for (const pair of subrules.items) colorRules(pair.value, true);
    const blacklist = doc.get('site-blacklist');
    if (isSeq(blacklist))
      for (const entry of blacklist.items) if (isMap(entry)) colorRules(entry.get('rules'), false);
    const providers = doc.get('rule-providers');
    if (isMap(providers))
      for (const pair of providers.items)
        if (isMap(pair.value) && pair.value.get('behavior') === 'classical')
          colorRules(pair.value.get('payload'), false);
    decorations.set([
      ...ruleColors,
      ...[...ranges]
        .filter(([s]) => !s.declaration && names.get(s.kind)?.has(s.name))
        .map(([s, range]) => ({
          range,
          options: {
            inlineClassName: 'yaml-template-reference',
            hoverMessage: { value: labels[s.kind] + '引用 · F2 重命名' },
            stickiness: monaco.editor.TrackedRangeStickiness.NeverGrowsWhenTypingAtEdges,
          },
        })),
    ]);
  }
  refresh();
  const at = (position: monaco.IPosition) =>
    [...ranges].find(([, range]) => range.containsPosition(position));
  const selector = { language: 'yaml', pattern: model.uri.path };
  const completion = monaco.languages.registerCompletionItemProvider(selector, {
    triggerCharacters: [',', ':', ' '],
    provideCompletionItems(current, position) {
      if (current !== model) return { suggestions: [] };
      const hit = at(position);
      if (!hit || hit[0].declaration) return { suggestions: [] };
      const [s, range] = hit;
      const values = new Set(
        s.kind === 'groupType'
          ? groupTypes
          : s.kind === 'proxyType'
            ? proxyTypes
            : names.get(s.kind),
      );
      if (s.kind === 'proxy') builtins.forEach((v) => values.add(v));
      return {
        suggestions: [...values].map((value) => {
          let insertText = value;
          if (s.node.type === 'QUOTE_DOUBLE') insertText = JSON.stringify(value).slice(1, -1);
          else if (s.node.type === 'QUOTE_SINGLE') insertText = value.replaceAll("'", "''");
          else if (s.start === 0 && s.end === String(s.node.value ?? '').length)
            insertText = stringify(value, {
              lineWidth: 0,
              collectionStyle: 'flow',
            }).trimEnd();
          return {
            label: value,
            detail: labels[s.kind],
            kind: monaco.languages.CompletionItemKind.Reference,
            insertText,
            range,
            sortText: '0' + value,
          };
        }),
      };
    },
  });
  const rename = monaco.languages.registerRenameProvider(selector, {
    resolveRenameLocation(current, position) {
      const hit = current === model && at(position);
      if (hit && names.get(hit[0].kind)?.has(hit[0].name))
        return { range: hit[1], text: hit[0].name };
      return {
        range: monaco.Range.fromPositions(position),
        text: '',
        rejectReason: '请选择本模板声明的名称或其引用',
      };
    },
    provideRenameEdits(current, position, newName) {
      const hit = current === model && at(position);
      if (!hit) return { edits: [] };
      const [s] = hit;
      if (
        !newName.trim() ||
        newName !== newName.trim() ||
        /[,():\r\n]/.test(newName) ||
        names.get(s.kind)?.has(newName) ||
        (s.kind === 'proxy' && builtins.includes(newName))
      )
        return { edits: [], rejectReason: '请使用唯一且不含规则分隔符的名称' };
      const doc = parseDocument(model.getValue());
      if (doc.errors.length) return { edits: [], rejectReason: '请先修复 YAML 语法' };
      replaceSymbol(templateSymbols(doc), s.kind, s.name, newName);
      return {
        edits: [
          {
            resource: model.uri,
            versionId: model.getVersionId(),
            textEdit: {
              range: model.getFullModelRange(),
              text: doc.toString(),
            },
          },
        ],
      };
    },
  });
  const keys = instance.onKeyDown((e) => {
    if (
      instance.getOption(monaco.editor.EditorOption.readOnly) ||
      e.ctrlKey ||
      e.metaKey ||
      e.altKey ||
      e.shiftKey
    )
      return;
    if (e.keyCode !== monaco.KeyCode.Backspace && e.keyCode !== monaco.KeyCode.Delete) return;
    const selection = instance.getSelection();
    if (!selection || instance.getSelections()?.length !== 1) return;
    let remove: monaco.Range = selection;
    if (selection.isEmpty()) {
      const hit = at(selection.getPosition());
      if (!hit) return;
      const [s, range] = hit;
      if (s.declaration || !names.get(s.kind)?.has(s.name) || range.isEmpty()) return;
      const boundary =
        e.keyCode === monaco.KeyCode.Backspace ? range.getStartPosition() : range.getEndPosition();
      if (selection.getPosition().equals(boundary)) return;
      remove = range;
    } else {
      for (const [s, range] of ranges) {
        if (s.declaration || !names.get(s.kind)?.has(s.name)) continue;
        const overlap = range.intersectRanges(selection);
        if (overlap && !overlap.isEmpty()) remove = remove.plusRange(range);
      }
      if (remove.equalsRange(selection)) return;
    }
    e.preventDefault();
    e.stopPropagation();
    instance.pushUndoStop();
    instance.executeEdits('remove-reference', [{ range: remove, text: '' }]);
    instance.pushUndoStop();
  });
  const listener = model.onDidChangeContent(refresh);
  return () => {
    completion.dispose();
    rename.dispose();
    keys.dispose();
    listener.dispose();
    decorations.clear();
  };
}
