const nameToken = String.raw`\s*(?:"(?:\\.|[^"\\])*"|[^,"\[\]\r\n]+)\s*`;
const linePattern = new RegExp(String.raw`^(.*),\s*\[((?:${nameToken}(?:,${nameToken})*)?)\]\s*$`);

export function parseBlacklistRule(line: string) {
  const match = linePattern.exec(line);
  if (!match || /[\r\n]/.test(line)) throw new Error('黑名单规则格式应为 类型,内容,[节点A, 节点B]');
  const rule = match[1].trim();
  const start = match[0].lastIndexOf(match[2] + ']');
  const tokens = [...match[2].matchAll(new RegExp(nameToken, 'g'))];
  const slots = tokens.map((token) => {
    const raw = token[0].trim();
    const name: unknown = raw.startsWith('"') ? JSON.parse(raw) : raw;
    if (typeof name !== 'string' || !name.trim() || /[\r\n]/.test(name))
      throw new Error('黑名单节点名称不能为空');
    return {
      name,
      start: start + token.index! + token[0].indexOf(raw),
      end: start + token.index! + token[0].indexOf(raw) + raw.length,
    };
  });
  if (new Set(slots.map((slot) => slot.name)).size !== slots.length)
    throw new Error('黑名单节点不能重复');
  return { rule, proxies: slots.map((slot) => slot.name), slots };
}

export function blacklistProxyToken(name: string) {
  return /[,"[\]\\\r\n]/.test(name) || name !== name.trim() ? JSON.stringify(name) : name;
}

export function formatBlacklistRule(rule: string, proxies: string[]) {
  const line = `${rule},[${proxies.map(blacklistProxyToken).join(', ')}]`;
  parseBlacklistRule(line);
  return line;
}
