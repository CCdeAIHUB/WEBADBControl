# Windows → Web 功能对照表

| Windows 能力 | Web 落点 | 后端路径 |
|---|---|---|
| 设备发现、USB/无线连接 | 工作台、设备中心 | `GET /devices`、`POST /devices/connect` |
| 设备详情与状态 | 设备详情 / 概览 | `GET /devices/{id}/overview` |
| 实时画面与触控 | scrcpy H.264 视频流（可显式启停、选择 15/30/60 FPS、连续触控） | WebSocket `/screen?fps=`、scrcpy 控制通道、`POST /actions` |
| 返回、主页、多任务、电源、音量 | 屏幕控制栏 | 固定 ADB 参数白名单；OEM 拒绝 `INJECT_EVENTS` 时主页走 HOME Intent，返回/多任务/触摸走伴侣无障碍 |
| 硬件与电池信息 | 硬件信息 | `getprop`、`dumpsys battery` |
| 应用列表、启动、停止、清除、卸载、安装 | 应用管理 | `/packages/*` |
| 文件浏览、上传、下载 | 文件管理 | `/files/*` |
| ADB 终端 | 安全终端 | `args: string[]`，不经过主机 Shell |
| Companion 安装、能力与权限 | 伴侣能力 | Core `device.*` IPC |
| 相机、录音、剪贴板、短信、电话、传感器、音量、悬浮窗等 | 通用能力调用器 | Core `device.invoke` → QUIC commandRequest |
| 自动化任务 DSL、任务 CRUD | 自动化任务 | SQLite + `/automation/tasks` |
| 手动运行、暂停、继续、停止、运行记录 | 自动化任务 | 显式状态机 + `/automation/runs` |
| AI 多模型、视觉输入、流式对话 | AI 助手 | 服务端 OpenAI 兼容流代理 |
| 主题、刷新、模型配置 | 系统设置 | 服务端安全配置存储 |
| 内置管理员、远程用户与设备授权 | 系统设置 / 远程用户与设备权限 | Web `/users/*` → Core `remote.admin.*` |
| 默认管理员密码与首次登录强制改密 | 登录 / 修改密码 | `/session`、`/password` → Core `remote.admin.*` |
| 加密远程控制监听 | 系统设置 / 远程控制服务 | Core QUIC/TLS 1.3 UDP，默认关闭，重启生效 |
| 请求日志、关键操作审计、前端异常上报 | 系统日志 | `/logs`、`/logs/stats`、`/logs/client-error` |
| 结构化错误、traceId、恢复建议 | 全局通知与错误状态 | 统一 `AppError` |

原 Rust Core、Android Companion、scrcpy 4.0、QUIC 协议与许可证文件位于 `core/`，没有以 Go 逻辑替代。

## 投屏链路差异（2026-09 更新）

Windows 客户端的 `ProjectionSession` 可选择 ADB scrcpy 或 Companion QUIC，支持 480p/720p/1080p、0.5–20 Mbps、30/45/60 FPS，并使用连续触控事件。Web 端现已通过 Core `adb.scrcpy.start/stop` 管理相同 scrcpy 4.0 服务，由 Go 网关转发带帧元数据的 H.264 包和连续触控事件。

浏览器在安全上下文优先使用 WebCodecs；普通局域网 HTTP 自动使用 TinyH264 软件解码。为保证两条路径收到相同的兼容码流，Web scrcpy 会话固定请求 AVC Baseline Level 4。服务端日志记录会话尺寸、首个配置包、首个关键帧和结束时的包计数，浏览器解码异常进入系统日志。当前 Web 暂未开放 Windows 客户端的码率、分辨率和 Companion QUIC 视频源选择。
