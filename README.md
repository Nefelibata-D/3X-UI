# 3X-UI 项目定制版

基于 [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) 二次开发，项目仓库为 [Nefelibata-D/3X-UI](https://github.com/Nefelibata-D/3X-UI)。沿用官方 React、TypeScript、Ant Design 前端及 Go 后端，在原有节点、用户、协议和订阅能力上增加项目功能。

## 项目功能

- 负载均衡：节点额度池、客户端节点额度与权重、独立每月周期及 TCP 网关策略。
- 定制订阅：规则集、模板和客户端订阅配置，配合定制 Mihomo 客户端使用。
- 通行密钥：支持多个密钥及域名配置，正式部署使用 HTTPS。
- 界面精简：移除 API 文档、赞助、Telegram/Discord 设置入口及首页、登录广告；保留邮件、订阅和协议配置。

基础登录、用户凭据、节点管理、Xray 与基础订阅继续复用上游实现。页面隐藏不代表对应后台功能禁用。实现边界见 [项目定制说明](README.project.md)、[项目模块说明](internal/clash/README.md)。

## 支持范围

本项目仅发布 **Linux amd64（x86_64）** 安装包，面向 **Ubuntu / Debian**，使用 systemd 管理服务。采用静态链接构建；具体系统版本兼容性仍需部署验证。不发布 Windows、ARM 或其他架构安装包，也不发布 Docker 镜像。

## 安装

在 Ubuntu / Debian 的 root Shell 中执行，需预先安装 `curl`。安装脚本及安装包均来自本项目仓库。

稳定版（仓库需已有正式 Release）：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Nefelibata-D/3X-UI/main/install.sh)
```

安装指定版本时，在命令末尾传入 [Releases](https://github.com/Nefelibata-D/3X-UI/releases) 中实际存在的版本标签。安装完成后按终端输出访问面板，通过 `x-ui` 打开命令行管理菜单。

## 更新

面板更新与 `x-ui` 菜单均使用本项目更新源：稳定通道读取最新正式 Release；本仓库只发布版本标签对应的正式版本。更新包提供对应 `.sha256` 校验文件。Xray 等独立组件保留各自的上游更新源。

子节点远程更新调用子节点自身更新器；子节点也需要先安装包含本项目更新源的版本。

## 自动构建与发布

[GitHub Actions](https://github.com/Nefelibata-D/3X-UI/actions) 按仓库身份控制运行：

- 本仓库保留常规 CI：Go 与前端检查、测试和构建。
- 推送 `v*` 版本标签后自动构建并发布正式 Release，标题就是标签名，例如 `v3.11.0.1`。
- 标签必须与 `internal/config/version` 一致，否则构建失败。
- 手动重跑时选择对应版本标签，也会自动上传 Release；普通分支不执行发布。
- 发布产物只有 `x-ui-linux-amd64.tar.gz` 和 `x-ui-linux-amd64.tar.gz.sha256`。
- Docker 发布、文档 CI/Pages、Claude PR/Issue、定时变异测试、单独 CodeQL 和缓存清理仅允许在官方上游仓库运行，在本仓库跳过。事件仍可能生成已跳过的工作流记录。

以上是工作流配置，不代表已经成功发布或完成真实服务器安装验证。

## 开发与许可证

唯一前端位于 `frontend/`，项目扩展位于 `internal/clash/`。Go module 路径保留上游命名，避免无关的大范围改动。本地 Docker 源码开发环境与 Docker 镜像发布相互独立。

项目遵循 [GPL-3.0](LICENSE)，保留上游版权与许可证。
