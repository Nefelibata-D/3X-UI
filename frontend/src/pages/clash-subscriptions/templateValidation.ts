import { loadBalanceErrors } from "./loadBalanceModel";
import validateSchema from "virtual:template-schema-validator";
import { isMap, isSeq, isNode, parseDocument } from "yaml";
import { builtins, templateSymbols } from "./templateSymbols";

export interface DependencyNode {
  name: string;
  kind: "proxy" | "group";
  index: number;
  dependencies: string[];
  via?: string;
  pending: boolean;
}
export function templateDependencies(content: string) {
  const doc = parseDocument(content);
  const nodes: DependencyNode[] = [];
  if (!isMap(doc.contents)) return { doc, nodes };
  for (const collection of ["proxies", "proxy-groups"]) {
    const list = doc.get(collection);
    if (!isSeq(list)) continue;
    list.items.forEach((item, index) => {
      if (!isMap(item) || typeof item.get("name") !== "string") return;
      const name = String(item.get("name"));
      const via = item.get("dialer-proxy");
      const members = item.get("proxies");
      const use = item.get("use");
      const dependencies = isSeq(members) ? members.items.map(String) : [];
      const group = collection === "proxy-groups";
      nodes.push({
        name,
        index,
        kind: group ? "group" : "proxy",
        via: typeof via === "string" ? via : undefined,
        dependencies: group
          ? dependencies
          : typeof via === "string"
            ? [via]
            : [],
        pending:
          group &&
          !dependencies.length &&
          !(isSeq(use) && use.items.length) &&
          !item.get("include-all") &&
          !item.get("include-all-proxies") &&
          !item.get("include-all-providers"),
      });
    });
  }
  return { doc, nodes };
}
export function dependencyCycle(nodes: DependencyNode[]): string[] | undefined {
  const graph = new Map(nodes.map((n) => [n.name, n.dependencies]));
  const active = new Set<string>();
  const done = new Set<string>();
  const path: string[] = [];
  function visit(name: string): string[] | undefined {
    if (active.has(name)) return [...path.slice(path.indexOf(name)), name];
    if (done.has(name)) return;
    active.add(name);
    path.push(name);
    for (const next of graph.get(name) ?? []) {
      const cycle = visit(next);
      if (cycle) return cycle;
    }
    path.pop();
    active.delete(name);
    done.add(name);
  }
  for (const name of graph.keys()) {
    const cycle = visit(name);
    if (cycle) return cycle;
  }
}
export function connectProxy(
  content: string,
  source: string,
  target: string | null,
) {
  const { doc, nodes } = templateDependencies(content);
  if (doc.errors.length) throw Error(doc.errors[0].message);
  if (new Set(nodes.map((n) => n.name)).size !== nodes.length)
    throw Error("代理或策略组名称重复，无法连线");
  const from = nodes.find((n) => n.name === source);
  if (!from || from.kind !== "proxy")
    throw Error("请从出口代理连接到前置代理或策略组");
  if (target !== null) {
    if (!nodes.some((n) => n.name === target) && !builtins.includes(target))
      throw Error("前置代理不存在");
    from.dependencies = [target];
    const cycle = dependencyCycle(nodes);
    if (cycle) throw Error("循环依赖：" + cycle.join(" → "));
    doc.setIn(["proxies", from.index, "dialer-proxy"], target);
  } else doc.deleteIn(["proxies", from.index, "dialer-proxy"]);
  return doc.toString();
}
export interface TemplateReport {
  diagnostics: { message: string; start: number; end: number }[];
  errors: string[];
  warnings: string[];
  pending: string[];
}
export function validateTemplate(
  content: string,
  stage: "template" | "subscription" = "template",
  knownProxies: string[] = [],
): TemplateReport {
  const report: TemplateReport = {
    errors: [],
    warnings: [],
    pending: [],
    diagnostics: [],
  };
  const { doc, nodes } = templateDependencies(content);
  const add = (message: string, node: unknown = doc.contents) => {
    const range = isNode(node) ? node.range : undefined;
    report.errors.push(message);
    report.diagnostics.push({
      message,
      start: range?.[0] ?? 0,
      end: range?.[1] ?? 1,
    });
  };
  for (const e of doc.errors) {
    report.errors.push(e.message);
    report.diagnostics.push({
      message: e.message,
      start: e.pos[0],
      end: e.pos[1],
    });
  }
  if (report.errors.length) return report;
  if (!isMap(doc.contents)) {
    add("配置必须是 YAML 对象");
    return report;
  }
  let data: unknown;
  try {
    data = doc.toJS({ maxAliasCount: 100 });
  } catch (e) {
    add(String(e));
    return report;
  }
  if (!validateSchema(data))
    for (const e of validateSchema.errors ?? []) {
      const path = e.instancePath
        .split("/")
        .slice(1)
        .map((key) => key.replace(/~1/g, "/").replace(/~0/g, "~"));
      add((e.instancePath || "/") + " " + e.message, doc.getIn(path, true));
    }
  loadBalanceErrors(content, stage, add);
  const seen = new Set<string>();
  for (const node of nodes) {
    const source = doc.getIn(
      [node.kind === "group" ? "proxy-groups" : "proxies", node.index],
      true,
    );
    if (seen.has(node.name) || builtins.includes(node.name))
      add(
        "代理名称重复或占用内置名称：" + node.name,
        isMap(source) ? source.get("name", true) : source,
      );
    seen.add(node.name);
    if (node.kind === "group" && node.via !== undefined)
      add(
        node.name + "：策略组不能设置 dialer-proxy",
        isMap(source) ? source.get("dialer-proxy", true) : source,
      );
    if (node.pending) report.pending.push(node.name);
  }
  const slots = templateSymbols(doc);
  const providers = doc.get("proxy-providers");
  const blacklists = doc.get("site-blacklist");
  const exampleNodes = new Set(
    isSeq(blacklists)
      ? blacklists.items.filter(isMap).map((b) => b.get("proxy", true))
      : [],
  );
  for (const s of slots.filter(
    (s) => !s.declaration && s.kind !== "groupType" && s.kind !== "proxyType",
  )) {
    if (stage === "template" && exampleNodes.has(s.node)) continue;
    if (s.kind === "proxy" && (builtins.includes(s.name) || knownProxies.includes(s.name))) continue;
    if (
      !slots.some(
        (d) => d.declaration && d.kind === s.kind && d.name === s.name,
      )
    ) {
      if (s.kind === "proxy" && isMap(providers) && providers.items.length)
        report.warnings.push("代理引用需等待集合加载后验证：" + s.name);
      else add("引用不存在：" + (s.name || "（空）"), s.node);
    }
  }
  const cycle = dependencyCycle(nodes);
  if (cycle)
    for (const slot of slots.filter(
      (s) => s.kind === "proxy" && cycle.includes(s.name),
    ))
      add("循环依赖：" + cycle.join(" → "), slot.node);
  if (stage === "subscription" && report.pending.length)
    for (const node of nodes.filter((n) => n.pending))
      add(
        "下发前必须完成节点注入：" + node.name,
        doc.getIn(["proxy-groups", node.index], true),
      );
  report.errors = [...new Set(report.errors)];
  return report;
}
