# 3X-UI 定制版

基于 [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) 二次开发的 Xray 管理面板，保留上游的多协议配置、多节点管理、用户流量统计和订阅能力，增加面向定制 Mihomo 客户端的负载均衡、节点额度管理和订阅配置。

## 特色功能

- **节点额度与权重**：配置节点额度池，为用户分配各节点的流量限额和权重，支持独立的每月重置周期。
- **客户端负载均衡**：配合定制 Mihomo 客户端，按权重选择出口并保持连接分配的黏性，由客户端直接连接出口节点。
- **定制订阅**：管理规则集和完整 YAML 模板，为用户配置代理组、节点注入、路由、黑名单及链式代理。
- **通行密钥登录**：支持添加多个通行密钥，正式域名使用 HTTPS。
- **精简界面**：精简广告、赞助及部分集成入口，保留协议配置、邮件和订阅功能。

加权策略和额度联动需要配套的定制 Mihomo 客户端。基础面板功能可参考 [上游项目](https://github.com/MHSanaei/3x-ui)。

## 安装

提供 **Linux amd64（x86_64）** 安装包，面向使用 systemd 的 **Ubuntu / Debian**。

在 root Shell 中执行以下命令，需预先安装 `curl`：

```bash
bash <(curl -fsSL https://raw.githubusercontent.com/Nefelibata-D/3X-UI/main/install.sh)
```

安装包见 [Releases](https://github.com/Nefelibata-D/3X-UI/releases)。需要指定版本时，在安装命令末尾追加对应版本标签。

安装完成后，按终端输出访问面板。运行 `x-ui` 可打开管理菜单，管理服务、登录信息和证书。

## 更新

通过面板或 `x-ui` 菜单更新，安装包来自本项目的正式 Release。Xray 等独立组件仍使用各自的上游更新源。

使用多节点管理时，子节点也需安装本定制版，才能使用扩展功能及本项目更新源。

## 致谢与许可证

感谢 [MHSanaei/3x-ui](https://github.com/MHSanaei/3x-ui) 及其贡献者。本项目遵循 [GPL-3.0](LICENSE)，保留上游版权与许可证。
