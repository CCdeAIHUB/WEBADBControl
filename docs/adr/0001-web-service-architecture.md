# ADR 0001：原 Rust Core 之上的 Go Web 服务

## 背景

ADBControl Windows 客户端已经围绕 Rust Core、ADB Provider、Android Companion QUIC 协议和 scrcpy 建立稳定边界。Web 版本需要网络访问、浏览器媒体适配、服务端持久化与 Linux 部署，但不能形成第二套设备核心。

## 决策

1. 原 `adbcontrol-core` 源码、Android Companion 与 scrcpy 源码完整保留在独立仓库的 `core/`。
2. Go 服务以 JSON Lines stdio 子进程适配原 Core，所有 Core 响应严格按请求 ID 关联。
3. ADB 设备动作由 Go 映射为参数数组后调用 `adb.exec`，不执行任意主机 Shell。
4. 浏览器通过同源 HTTP 与 WebSocket 使用设备、任务、AI 和设置能力。
5. 自动化任务延续版本化 JSON DSL、SQLite 持久化和显式运行状态机。
6. 浏览器使用密码登录和 HttpOnly 随机会话；默认密码仅用于首次启动并强制修改，兼容令牌只服务于脚本/API 客户端。

## 原因

- 保持原核心行为与协议一致，Web、Windows 和未来前端可以共享能力事实源。
- Go 标准库适合构建可移植的单进程网络服务，并能清晰管理 HTTP 生命周期。
- 同源部署减少浏览器跨域、密钥泄露与 WebSocket 鉴权风险。

## 替代方案

- 用 Go 重写 ADB/Companion 核心：拒绝，会导致协议和安全边界漂移。
- 浏览器直接 WebUSB：拒绝作为主路径，无法覆盖 Firefox、远程 Linux 主机和 Companion 服务端能力。
- 继续使用 Windows UI 服务：拒绝，无法在主流 Linux 发行版运行。

## 影响范围

新增 Go API、Vue Web UI、SQLite 任务仓储、Linux/Docker 部署与 CI；原 IPC 与 QUIC v1 保持兼容。

## 风险

浏览器不能直接消费 Windows D3D 交换链；当前媒体网关以安全截图帧 WebSocket 作为通用路径，后续可在不修改页面 API 的前提下增加 H.264/WebCodecs 传输。

## 未来演进

为 Core 增加二进制流 IPC、浏览器 H.264 解码、服务端多用户 RBAC 与审计导出。
