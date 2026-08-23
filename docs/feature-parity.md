# Windows → Web 功能对照表

| Windows 能力 | Web 落点 | 后端路径 |
|---|---|---|
| 设备发现、USB/无线连接 | 工作台、设备中心 | `GET /devices`、`POST /devices/connect` |
| 设备详情与状态 | 设备详情 / 概览 | `GET /devices/{id}/overview` |
| 实时画面与触控 | 实时控制 | WebSocket `/screen`、`POST /actions` |
| 返回、主页、多任务、电源、音量 | 屏幕控制栏 | 固定 ADB 参数白名单 |
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
| 请求日志、关键操作审计、前端异常上报 | 系统日志 | `/logs`、`/logs/stats`、`/logs/client-error` |
| 结构化错误、traceId、恢复建议 | 全局通知与错误状态 | 统一 `AppError` |

原 Rust Core、Android Companion、scrcpy 4.0、QUIC 协议与许可证文件位于 `core/`，没有以 Go 逻辑替代。
