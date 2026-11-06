# 项目定制入口

本目录是完整官方 3x-ui 工作树。前端直接使用 frontend/ 的 React + Ant Design，后端基础能力保持官方语义；额外业务集中 internal/clash/。

安全设置增加通行密钥标签及登录入口，支持多个密钥和一个可自动识别的域名；更换域名会清空旧密钥。实现与验证边界见 [通行密钥说明](internal/clash/passkey/README.md)。本地请通过 localhost 访问，正式域名使用 HTTPS。

项目说明与 Git 现状见 [根 README](../README.md)，架构、需求与阶段见 [管理端文档](../.codex/plan/管理端/README.md)，本地运行见 [docker/dev](../docker/dev/README.md)。

当前界面定制：移除 API 文档、赞助商、Telegram/Discord 设置入口及首页/登录广告；保留邮件、订阅和官方协议配置。对应后端服务未禁用。internal/clash/ 已实现节点额度池、独立每月周期、客户端节点额度/权重及 TCP 网关策略，实际契约与本轮验证见 internal/clash/README.md。
项目新增功能统一命名为“负载均衡”，已接入官方侧边栏一级菜单，负责节点组、权重、用户分配及定制订阅，由 Mihomo 客户端选出口。官方 Xray 出站负载均衡页面、菜单和搜索入口已移除，项目不启用该中转能力；保留上游核心兼容代码，页面移除不代表后端能力封禁。

面板更新源统一为 https://github.com/Nefelibata-D/3X-UI：版本查询、网页更新脚本、安装/更新包与校验文件、命令行菜单及服务文件均从该仓库读取。稳定通道使用 latest release，开发通道使用 dev-latest，脚本主分支为 main；需先在该仓库发布对应版本及原格式的 x-ui-linux-<arch>.tar.gz 产物。子节点远程更新调用子节点自身更新器，因此子节点也需部署包含此修改的版本。Xray 等独立组件仍使用各自原更新源。Docker 源码开发环境继续按 docker/dev/README.md 更新。
