import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from 'react';
import * as monaco from 'monaco-editor';
import nightOwl from '@/components/clash/subscription/themes/night-owl.json';
import lightOwl from '@/components/clash/subscription/themes/light-owl.json';
import 'monaco-editor/esm/vs/base/browser/ui/codicons/codiconStyles.js';
import { templateSchema } from '@/lib/clash/subscription/template/schema';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import YamlWorker from 'monaco-yaml/yaml.worker?worker';
import { configureMonacoYaml } from 'monaco-yaml';
import { useTheme } from '@/hooks/useTheme';
import { installTemplateReferences } from '@/lib/clash/subscription/template/referenceEditor';
import { validateTemplate } from '@/lib/clash/subscription/template/validation';
import '@/components/clash/subscription/ClashYamlEditor.css';

(globalThis as typeof globalThis & { MonacoEnvironment: monaco.Environment }).MonacoEnvironment = {
  getWorker(_id, label) {
    return label === 'yaml' ? new YamlWorker() : new EditorWorker();
  },
};
const yamlSupport = configureMonacoYaml(monaco, {
  validate: true,
  completion: true,
  hover: true,
  format: true,
  enableSchemaRequest: false,
  schemas: [
    {
      uri: 'https://github.com/MetaCubeX/meta-json-schema/schemas/meta-json-schema.json',
      fileMatch: ['**/*.yaml', '**/*.yml'],
      schema: templateSchema,
    },
  ],
});
if (import.meta.hot) import.meta.hot.dispose(() => yamlSupport.dispose());

monaco.editor.defineTheme('night-owl', { ...nightOwl, base: 'vs-dark' });
monaco.editor.defineTheme('light-owl', { ...lightOwl, base: 'vs' });

export interface ClashYamlEditorProps {
  value: string;
  knownProxies?: string[];
  onChange?: (next: string) => void;
  minHeight?: string;
  maxHeight?: string;
  readOnly?: boolean;
  fontSize?: number;
}
export interface ClashYamlEditorHandle {
  focus: () => void;
}

const ClashYamlEditor = forwardRef<ClashYamlEditorHandle, ClashYamlEditorProps>(
  function ClashYamlEditor(
    {
      value,
      knownProxies = [],
      onChange,
      minHeight = '540px',
      maxHeight = '65vh',
      readOnly = false,
      fontSize = 14,
    },
    ref,
  ) {
    const host = useRef<HTMLDivElement>(null);
    const editor = useRef<monaco.editor.IStandaloneCodeEditor | null>(null);
    const shell = useRef<HTMLDivElement>(null);
    const [contextMenu, setContextMenu] = useState<{
      x: number;
      y: number;
      hasSelection: boolean;
    }>();
    const change = useRef(onChange);
    const known = useRef(knownProxies);
    useEffect(() => {
      known.current = knownProxies;
    }, [knownProxies]);
    const syncingValue = useRef(false);
    useEffect(() => {
      change.current = onChange;
    }, [onChange]);
    const initial = useRef({ value, readOnly });
    const { isDark } = useTheme();
    useImperativeHandle(ref, () => ({ focus: () => editor.current?.focus() }), []);
    useEffect(() => {
      if (!host.current) return;
      const model = monaco.editor.createModel(
        initial.current.value,
        'yaml',
        monaco.Uri.parse('inmemory://templates/' + crypto.randomUUID() + '.yaml'),
      );
      const instance = monaco.editor.create(host.current, {
        model,
        automaticLayout: true,
        readOnly: initial.current.readOnly,
        fontSize: 14,
        fontFamily: 'JetBrains Mono Variable',
        fontWeight: '400',
        lineHeight: 22,
        tabSize: 2,
        insertSpaces: true,
        minimap: { enabled: true },
        scrollBeyondLastLine: false,
        padding: { top: 12, bottom: 12 },
        wordWrap: 'on',
        folding: true,
        contextmenu: false,
        renderValidationDecorations: 'on',
        quickSuggestions: { other: true, strings: true, comments: false },
        renderWhitespace: 'selection',
        ariaLabel: 'YAML 模板编辑器',
      });
      editor.current = instance;
      const disposeReferences = installTemplateReferences(instance, model);
      const updateDiagnostics = () => {
        const report = validateTemplate(model.getValue(), 'template', known.current);
        monaco.editor.setModelMarkers(
          model,
          'template-validation',
          report.diagnostics.map((issue) => {
            const start = model.getPositionAt(issue.start);
            const end = model.getPositionAt(Math.max(issue.start + 1, issue.end));
            return {
              message: issue.message,
              severity: monaco.MarkerSeverity.Error,
              startLineNumber: start.lineNumber,
              startColumn: start.column,
              endLineNumber: end.lineNumber,
              endColumn: end.column,
            };
          }),
        );
      };
      updateDiagnostics();
      let validationTimer: ReturnType<typeof setTimeout>;
      const listener = instance.onDidChangeModelContent(() => {
        clearTimeout(validationTimer);
        validationTimer = setTimeout(updateDiagnostics, 200);
        if (!syncingValue.current) change.current?.(model.getValue());
      });
      return () => {
        clearTimeout(validationTimer);
        disposeReferences();
        listener.dispose();
        instance.dispose();
        model.dispose();
        editor.current = null;
      };
    }, []);
    useEffect(() => {
      const instance = editor.current;
      const model = instance?.getModel();
      if (instance && model && model.getValue() !== value) {
        syncingValue.current = true;
        try {
          model.pushStackElement();
          model.pushEditOperations(
            [],
            [{ range: model.getFullModelRange(), text: value }],
            () => null,
          );
          model.pushStackElement();
        } finally {
          syncingValue.current = false;
        }
      }
    }, [value]);
    useEffect(() => {
      editor.current?.updateOptions({ fontSize, lineHeight: Math.round(fontSize * 1.6) });
      let disposed = false;
      void document.fonts.load(`${fontSize}px "JetBrains Mono Variable"`).then(() => {
        if (disposed) return;
        monaco.editor.remeasureFonts();
        editor.current?.layout();
      });
      return () => {
        disposed = true;
      };
    }, [fontSize]);
    useEffect(() => {
      editor.current?.updateOptions({ readOnly });
    }, [readOnly]);
    useEffect(() => {
      monaco.editor.setTheme(isDark ? 'night-owl' : 'light-owl');
    }, [isDark]);
    function runContextAction(action: string) {
      const instance = editor.current;
      if (!instance) return;
      setContextMenu(undefined);
      if (action === 'undo' || action === 'redo') {
        instance.trigger('contextmenu', action, null);
        instance.focus();
        return;
      }
      instance.focus();
      void instance.getAction(action)?.run();
    }

    return (
      <div
        ref={shell}
        className="yaml-editor-shell"
        style={{ height: maxHeight, minHeight, width: '100%' }}
        onContextMenu={(event) => {
          event.preventDefault();
          const instance = editor.current;
          if (!instance || !shell.current) return;
          const selection = instance.getSelection();
          const position = instance.getTargetAtClientPoint(event.clientX, event.clientY)?.position;
          if ((!selection || selection.isEmpty()) && position) {
            instance.setPosition(position);
          }
          const rect = shell.current.getBoundingClientRect();
          setContextMenu({
            x: event.clientX - rect.left,
            y: event.clientY - rect.top,
            hasSelection: !!selection && !selection.isEmpty(),
          });
        }}
        onPointerDown={() => setContextMenu(undefined)}
      >
        <div ref={host} className="yaml-editor-host" />
        {contextMenu && (
          <div
            className="yaml-editor-context-menu"
            role="menu"
            aria-label="编辑器菜单"
            style={{ left: contextMenu.x, top: contextMenu.y }}
            onContextMenu={(event) => event.preventDefault()}
            tabIndex={-1}
            onPointerDown={(event) => event.stopPropagation()}
            onKeyDown={(event) => {
              if (event.key === 'Escape') {
                setContextMenu(undefined);
                editor.current?.focus();
              }
              if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
                event.preventDefault();
                const items = Array.from(
                  event.currentTarget.querySelectorAll<HTMLButtonElement>('button:not(:disabled)'),
                );
                const index = items.indexOf(document.activeElement as HTMLButtonElement);
                const next =
                  (index + (event.key === 'ArrowDown' ? 1 : -1) + items.length) % items.length;
                items[next]?.focus();
              }
            }}
          >
            {[
              {
                label: '插入引用 / 补全字段',
                action: 'editor.action.triggerSuggest',
                disabled: readOnly,
              },
              {
                label: '重命名引用（F2）',
                action: 'editor.action.rename',
                disabled: readOnly,
              },
              { separator: true },
              {
                label: '切换行注释（Ctrl+/）',
                action: 'editor.action.commentLine',
                disabled: readOnly,
              },
              { label: '撤销', action: 'undo', disabled: readOnly },
              { label: '重做', action: 'redo', disabled: readOnly },
              { separator: true },
              {
                label: '剪切',
                action: 'editor.action.clipboardCutAction',
                disabled: readOnly || !contextMenu.hasSelection,
              },
              {
                label: '复制',
                action: 'editor.action.clipboardCopyAction',
                disabled: !contextMenu.hasSelection,
              },
              {
                label: '粘贴',
                action: 'editor.action.clipboardPasteAction',
                disabled: readOnly,
              },
              { separator: true },
              {
                label: '全选',
                action: 'editor.action.selectAll',
                disabled: false,
              },
            ].map((item, index) =>
              'separator' in item ? (
                <div className="yaml-editor-context-separator" key={`separator-${index}`} />
              ) : (
                <button
                  key={item.action}
                  type="button"
                  role="menuitem"
                  disabled={item.disabled}
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => runContextAction(item.action)}
                >
                  {item.label}
                </button>
              ),
            )}
          </div>
        )}
      </div>
    );
  },
);
export default ClashYamlEditor;
