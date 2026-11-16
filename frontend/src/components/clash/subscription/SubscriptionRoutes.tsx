import { useState } from 'react';
import { Button, Checkbox, Space } from 'antd';
import { StopOutlined, UndoOutlined } from '@ant-design/icons';
import { stringify } from 'yaml';
import TemplateInsert from './TemplateInsert';
import type { Subscription, RuleSet } from '@/schemas/clash/subscription';
import {
  disabledField,
  documentObject,
  entries,
  ruleIdentity,
  setEntries,
  subscriptionFromYAML,
} from '@/lib/clash/subscription/model';

interface Props {
  section: 'rules' | 'rule-providers';
  base: string;
  binding: Subscription;
  combined: Record<string, unknown>;
  groups: string[];
  rules: RuleSet[];
  change: (next: Subscription) => void;
}
export default function SubscriptionRoutes({
  section,
  base,
  binding,
  combined,
  groups,
  rules,
  change,
}: Props) {
  const [showTemplate, setShowTemplate] = useState(true);
  const [showCustom, setShowCustom] = useState(true);
  const field = disabledField[section];
  const order = section === 'rules' ? 'ruleOrder' : 'providerOrder';
  const identity = (name: string) => (section === 'rules' ? ruleIdentity(name) : name);
  const overlay = documentObject(binding.content);
  const paused = documentObject(binding.pausedContent || '{}');
  const custom = new Map(entries(overlay, section));
  const dormant = new Map(entries(paused, section));
  const inherited = new Map(entries(documentObject(base), section));
  const active = entries(combined, section);
  const activeIds = new Set(active.map(([id]) => id));
  const disabled = [...new Set([...binding[field], ...dormant.keys()])].filter(
    (id) => !activeIds.has(id) && (dormant.has(id) || inherited.has(id)),
  );
  function toggle(id: string, enable: boolean) {
    const next = { ...binding };
    if (enable) {
      if (dormant.has(id)) custom.set(id, dormant.get(id));
      dormant.delete(id);
      next[field] = binding[field].filter((key) => key !== id);
    } else {
      if (custom.has(id)) dormant.set(id, custom.get(id));
      custom.delete(id);
      next[field] = [...new Set([...binding[field], id])];
      if (!next[order]?.length) next[order] = active.map(([key]) => key);
    }
    next.content = stringify(setEntries(overlay, section, [...custom]));
    next.pausedContent = stringify(setEntries(paused, section, [...dormant]));
    change(next);
  }
  return (
    <div className="subscription-route-panel">
      <div className="subscription-route-scroll">
        <TemplateInsert
          tab={section === 'rules' ? 'routes' : 'providers'}
          content={stringify(combined)}
          groups={groups}
          rules={rules}
          readonly={false}
          visible={(name) => (custom.has(identity(name)) ? showCustom : showTemplate)}
          entryClassName={(name) => (custom.has(identity(name)) ? 'subscription-custom-row' : '')}
          onDisable={(name) => toggle(identity(name), false)}
          onChange={(content) => {
            const next = subscriptionFromYAML(base, content, binding);
            const nextIds = entries(documentObject(content), section).map(([id]) => id);
            if (nextIds.some((id) => dormant.has(id) && !activeIds.has(id)))
              throw Error('该条目已禁用，请先恢复');
            if (nextIds.length > active.length) {
              const added = nextIds.filter((id) => !activeIds.has(id));
              if (added.length !== nextIds.length - active.length) throw Error('匹配条件重复');
              next[order] = [...added, ...nextIds.filter((id) => activeIds.has(id))];
            }
            change(next);
          }}
        />
        {disabled
          .filter((id) => (dormant.has(id) ? showCustom : showTemplate))
          .map((id) => (
            <div className="subscription-disabled-row" key={id}>
              <StopOutlined aria-label="已禁用" />
              <span>{section === 'rules' ? String(dormant.get(id) ?? inherited.get(id)) : id}</span>
              <Button
                type="text"
                size="small"
                icon={<UndoOutlined />}
                aria-label={'恢复 ' + id}
                onClick={() => toggle(id, true)}
              >
                恢复
              </Button>
            </div>
          ))}
      </div>
      <Space className="subscription-source-toggle subscription-route-filters">
        <Checkbox checked={showTemplate} onChange={(e) => setShowTemplate(e.target.checked)}>
          模板
        </Checkbox>
        <Checkbox checked={showCustom} onChange={(e) => setShowCustom(e.target.checked)}>
          自定义
        </Checkbox>
      </Space>
    </div>
  );
}
