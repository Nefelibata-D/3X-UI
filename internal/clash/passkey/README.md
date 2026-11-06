# 通行密钥

安全设置的“通行密钥”标签管理当前管理员的 WebAuthn 凭据，登录页提供免密码入口。支持多个密钥，但全局仅一个 RP 域名。未配置时界面识别当前 hostname，管理员保存后生效；不会因从另一个域名访问而自动更换配置。

域名只接受主机名，不含协议、端口或路径，统一转为小写 ASCII/IDNA。拒绝 IP 和公共后缀；正式访问必须 HTTPS，本地使用 `http://localhost:2053`。Origin 的主机必须与配置完全一致，允许该主机的不同 HTTPS 端口；每次 ceremony 固定其发起 Origin，跨端口响应不能复用挑战。前端使用浏览器原生 WebAuthn JSON API，需要支持该 API 的新版浏览器。

更换规范化后的域名，在同一数据库事务中清空所有管理员的凭据、随机用户句柄和待完成请求，递增配置版本；保存相同域名不清理。配置行写锁串行化域名修改、凭据删除、注册完成与签名计数更新。单个管理员只能列出、删除自己的凭据。管理 API 仅允许官方管理员 cookie session，拒绝 Bearer/mTLS API 身份，并复用官方 CSRF。

`go-webauthn/webauthn` 校验注册和登录，不自建密码账户。注册要求 resident key，登录要求用户验证（例如设备 PIN/生物识别）。挑战与 CSRF session 的哈希、Origin、操作类型、账户、配置版本绑定，五分钟过期，校验前原子消费，一次性使用；每个 session 每类仅保留一个待完成请求。注册还绑定账户 LoginEpoch。发现签名计数回退则拒绝；同步密钥的合法零计数由标准库按 WebAuthn 语义处理。凭据保存公钥及验证元数据，私钥保留在验证器中。Passkey 是独立登录方式，不再要求密码/TOTP；原密码与 TOTP 登录流程保留。

最小上游接线：`clash/store.go` 调用本包迁移；`controller/api.go` 注册管理路由，`controller/index.go` 注册匿名登录路由；独立 `controller/passkey.go` 只承担 HTTP/session/CSRF、官方登录限速和成功日志通知。项目表为 `clash_passkey_config/accounts/credentials/ceremonies`，不写第二份账户凭据权威数据。API 登记在官方 endpoints.ts。

定向测试在 `test/`，使用隔离 SQLite 和真实 ECDSA 签名，保持未跟踪。运行 `go test ./internal/clash/passkey/test`；本机使用根目录 `go-project.ps1`。测试覆盖域名变更清理、同域名保留、Origin/用户验证/签名/计数校验、session 绑定、一次性消费和跨用户删除拒绝。浏览器验证使用隔离面板与 Chromium 虚拟验证器，不能等同于实物 Windows Hello、手机或硬件密钥验收。

2026-11-06 实跑：上述 Go 测试及 `-race` 通过；生产前端构建、改动文件 oxlint/Ant Design 检查通过。独立临时 Docker 面板上完成浏览器注册、退出后仅用 Passkey 登录、最近使用时间回读、删除、重新注册后换域名清空；管理 API 缺少 CSRF 返回 403、匿名访问返回 401。2053 开发面板已重启加载实现，未替用户配置域名或注册密钥。

全局检查仍有既存失败：`ClientNodeResetMode.tsx:19` 类型错误；i18n dead-keys 的 `pages.clients.speed`、`pages.clients.remaining`、`pages.nodes.deleteConfirmContent`；route contract 缺少 rulesets/templates 的五个登记项。新增 Passkey 文件及路由不在失败列表中。未实跑 PostgreSQL 或实物验证器。

界面反馈：域名与识别、保存按钮同排；更换已有域名前弹窗确认清空旧密钥的风险，操作结果用 message 反馈。添加按钮位于列表右上方，先调用设备验证器，再弹窗输入名称并提交注册完成请求；取消命名不会保存服务端凭据。名称在 finish 阶段校验，begin 不要求名称。删除保留确认框。

本轮交互调整实跑：隔离浏览器确认设备验证后才出现命名弹窗，命名保存后列表显示对应名称；域名输入框与识别按钮同排对齐，更换域名出现风险确认框。更新后的注册 API 定向测试、生产构建、oxlint 与 Ant Design 检查通过；全局 typecheck 仍仅报上述既存错误。

通行密钥区域的容器与 Ant Design 组件统一使用中文无衬线字体、14px 正文和 400 字重；表头与表单标签使用一致的字号和文字色。域名输入框以 360px 为基准，不拉伸填满，按钮紧随右侧。收紧表格及空状态留白，保存按钮使用蓝色主按钮样式。样式仅作用于此区域。
