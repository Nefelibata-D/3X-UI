# 项目定制入口

本目录是完整官方 3x-ui 工作树。前端直接使用 frontend/ 的 React + Ant Design，后端基础能力保持官方语义；额外业务集中 internal/clash/。

开发固定在官方 `v3.11.0`（`c621ba90`）基线。`baseline/v3.11.0` 保留该提交；`main` 汇集定制快照 `a7a80e0e` 与后续优化，个人首个发布版本为 `v3.11.0.1`；开发分支 `feature/v3.11.0-project-refactor` 保留，不自动合并官方 main。快照保存源码及上游已跟踪的生成契约；本地数据库、缓存、构建目录和项目隔离测试不提交。

明确保留的官方行为差异：删除节点时清理主控上的入站副本、关联和项目策略，保留远端实际入站以及客户端累计流量。这是用户要求的定制行为，由 `internal/web/service/node_detach.go` 实现，不恢复官方的关联入站删除阻止条件。

共享订阅编辑器位于 `frontend/src/components/clash/subscription/`，纯解析与校验位于 `frontend/src/lib/clash/subscription/`，数据契约位于 `frontend/src/schemas/clash/subscription.ts`，查询和修改位于 `frontend/src/api/queries/useClashSubscriptions.ts`。页面只组合资源与交互；规则集、模板和客户端订阅共享资源缓存。


安全设置增加通行密钥标签及登录入口，支持多个密钥和一个可自动识别的域名；更换域名会清空旧密钥。实现与验证边界见 [通行密钥说明](internal/clash/passkey/README.md)。本地请通过 localhost 访问，正式域名使用 HTTPS。

项目说明与 Git 现状见 [根 README](../README.md)，架构、需求与阶段见 [管理端文档](../.codex/plan/管理端/README.md)，本地运行见 [docker/dev](../docker/dev/README.md)。

当前界面定制：移除 API 文档、赞助商、Telegram/Discord 设置入口及首页/登录广告；保留邮件、订阅和官方协议配置。对应后端服务未禁用。internal/clash/ 已实现节点额度池、独立每月周期、客户端节点额度/权重及 TCP 网关策略，实际契约与本轮验证见 internal/clash/README.md。
项目新增功能统一命名为“负载均衡”，已接入官方侧边栏一级菜单，负责节点组、权重、用户分配及定制订阅，由 Mihomo 客户端选出口。官方 Xray 出站负载均衡页面、菜单和搜索入口已移除，项目不启用该中转能力；保留上游核心兼容代码，页面移除不代表后端能力封禁。

面板更新源统一为 https://github.com/Nefelibata-D/3X-UI：版本查询、网页更新脚本、安装/更新包与校验文件、命令行菜单及服务文件均从该仓库读取。稳定通道使用 latest release，开发通道使用 dev-latest，脚本主分支为 main；需先在该仓库发布对应版本及原格式的 x-ui-linux-<arch>.tar.gz 产物。子节点远程更新调用子节点自身更新器，因此子节点也需部署包含此修改的版本。Xray 等独立组件仍使用各自原更新源。Docker 源码开发环境继续按 docker/dev/README.md 更新。

上游合并接点使用简短 `Project:` 注释，说明项目依赖、调用顺序或有意保留的行为差异，不逐行解释代码。主要接点为订阅生成、节点删除/同步、额度重置、Remote 请求策略、Web 生命周期、登录/安全设置、导航及缓存失效。静态资源适配集中在 `internal/web/assets_file.go`；官方入口只调用适配函数。小段表单和菜单逻辑保持就地，不为减少行数拆成无独立语义的组件。

路由名称沿用官方 `titleKey` 和所有 locale 的对应键；节点流量样式限定在项目组件内。翻译 JSON、依赖锁和生成 API/schema 文件不插入注释：分别通过路由键、依赖清单及源端契约追踪差异，合并后按官方工具重新生成。界面入口裁剪只影响界面，不表示禁用后端。

本轮（2026-11-06）定向检查：前端 typecheck、lint、导航契约及 Go 全包编译通过。翻译键集合一致性通过；dead-key 检查仍报告既有界面定制留下的 `pages.clients.speed`、`pages.clients.remaining` 和 `pages.nodes.deleteConfirmContent`，未通过扩展豁免或删除上游翻译来隐藏。未重跑真实跨容器连接验证。

发布配置：本仓库仅构建 Linux amd64，面向 Ubuntu / Debian；版本标签发布正式 Release，main 发布 dev-latest。Docker 发布及上游专用自动化通过仓库身份判断跳过，常规 CI 保留。安装与发布说明以本目录 README.md 为准。

本项目不启用 Dependabot 自动依赖升级；已移除 .github/dependabot.yml，依赖更新按实际需求手动进行。
