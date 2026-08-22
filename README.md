# ADBControl Web

ADBControl Web 是 ADBControl 的独立 Web 版本：保留原 Rust Core、Android Companion 与 scrcpy 能力，以 Go 提供安全网络服务，并使用 Vue 3、TypeScript 和 Tailwind CSS 构建现代化中文管理界面。

## 架构

```text
Browser (Vue 3 + TypeScript + Tailwind CSS)
    │ HTTP / WebSocket
Go Web Service
    ├── Auth / REST / screen gateway / AI proxy
    ├── SQLite automation runtime
    └── JSON Lines over stdio
Rust adbcontrol-core
    ├── ADB Provider
    └── Android Companion Provider ── QUIC ── Android Companion App
```

Go 网络层不会绕过 Core 创建第二套设备协议。ADB 命令仍以 `args: string[]` 进入 `adb.exec`，Companion 能力仍通过 `device.invoke` 转换为 QUIC `commandRequest`。

## 功能

- USB / 无线 ADB 设备发现、连接与在线验证；
- 实时屏幕、点击控制、导航键、音量与电源操作；
- 设备信息、电池、系统与硬件属性；
- 应用安装、启动、停止、清除数据和卸载；
- 文件浏览、上传与下载；
- 不经过主机 Shell 的 ADB 参数终端；
- Android Companion 安装、能力、权限和通用调用；
- SQLite 自动化任务、JSON DSL、运行状态与控制；
- OpenAI 兼容多模型、视觉附件与流式 AI 对话；
- 深色/浅色主题、响应式布局、结构化错误与访问鉴权。

完整映射见 [功能对照表](docs/feature-parity.md)。

## 本地开发

需要 Go 1.23+、Node.js 22+、Rust stable、ADB，以及 Android 设备或模拟器。

```bash
cd core
cargo build -p adbcontrol-core

cd ../server
go mod download
WEBADB_CORE_BINARY=../core/target/debug/adbcontrol-core go run ./cmd/webadbcontrol

cd ../web
pnpm install
pnpm dev
```

开发地址为 `http://127.0.0.1:5173`。Vite 会把 `/api` 和 WebSocket 转发到 Go 服务。

## Docker 部署

```bash
cp .env.example .env
# 浏览器首次登录默认密码为 admin，登录后会要求立即修改
docker compose up -d --build
```

打开 `http://服务器地址:8080`。容器需要访问 `/dev/bus/usb` 才能管理 USB 设备；无线 ADB 不需要 USB 映射。

## 主流 Linux 发行版

`deploy/install.sh` 支持使用以下包管理器安装 ADB 依赖：

- Debian / Ubuntu：`apt-get`
- Fedora / RHEL 系：`dnf`
- Arch Linux：`pacman`
- openSUSE：`zypper`

生产环境建议放在 Caddy、Nginx 或 Traefik 后提供 HTTPS，并设置 `WEBADB_BEHIND_PROXY=true`。首次登录默认密码为 `admin`，系统会引导立即修改；`WEBADB_AUTH_TOKEN` 仅作为脚本/API 客户端的可选兼容令牌。

## 验证

```bash
cd server && go test ./...
cd ../web && pnpm test && pnpm build
cd ../core && cargo test --workspace
```

CI 还会执行 Rust fmt、clippy、QUIC feature check 与 Docker 构建。

## 安全

- 不接受任意主机 Shell 命令；
- 高风险设备动作使用服务端白名单与前端二次确认；
- AI API Key 只保存在服务端数据目录；
- 管理密码使用随机盐派生后持久化，首次登录强制修改默认密码；
- 默认启用同源、CSP、Clickjacking 与 MIME 嗅探防护；
- 生产数据目录与 `.env` 不进入 Git。

## 许可证

原 Core、Android Companion、scrcpy 与 FFmpeg 的来源和许可证说明均保留在 `core/third_party/`。分发构建前请核对对应运行库的许可证义务。
