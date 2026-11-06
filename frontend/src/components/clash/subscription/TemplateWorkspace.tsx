import { useState, type ReactNode } from 'react';
import { Flex, Select, Tag, Typography } from 'antd';
import YamlEditor from '@/components/clash/subscription/ClashYamlEditor';
import '@/components/clash/subscription/Templates.css';

interface Props {
  content: string;
  knownProxies?: string[];
  onChange: (content: string) => void;
  readonly: boolean;
  error?: string;
  groupCount: number;
  children: ReactNode;
}

export default function TemplateWorkspace({
  content,
  knownProxies,
  onChange,
  readonly,
  error,
  groupCount,
  children,
}: Props) {
  const sizes = [12, 13, 14, 15, 16, 18, 20, 22, 24];
  const [fontSize, setFontSize] = useState(() => {
    const saved = Number(localStorage.getItem('clash-yaml-font-size'));
    return sizes.includes(saved) ? saved : 14;
  });
  return (
    <div className="clash-template-workspace">
      <section className="clash-template-code">
        <Flex justify="space-between" style={{ padding: '12px 16px' }}>
          <Typography.Text strong>基础 YAML</Typography.Text>
          <Flex align="center" gap={12}>
            <Flex align="center" gap={6}>
              <Typography.Text type="secondary">字号</Typography.Text>
              <Select
                aria-label="YAML 字号"
                size="small"
                value={fontSize}
                style={{ width: 84 }}
                options={sizes.map((size) => ({ value: size, label: `${size}px` }))}
                onChange={(size) => {
                  setFontSize(size);
                  localStorage.setItem('clash-yaml-font-size', String(size));
                }}
              />
            </Flex>
            <Tag color={error ? 'error' : 'success'}>
              {error ? '语法待修正' : `${groupCount} 个策略组`}
            </Tag>
          </Flex>
        </Flex>
        <YamlEditor
          value={content}
          knownProxies={knownProxies}
          fontSize={fontSize}
          onChange={onChange}
          minHeight="min(360px, 40vh)"
          maxHeight="min(600px, 59vh)"
          readOnly={readonly}
        />
      </section>
      <section className="clash-template-controls">{children}</section>
    </div>
  );
}
