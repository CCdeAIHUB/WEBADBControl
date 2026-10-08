# ADR 0004：ADB 引导、伴侣主动连接的独立 QUIC 链路

## 背景

Android 伴侣已经具备固定 Core 证书并主动建立 QUIC 的客户端实现，但 Web 部署此前只启动远程管理 QUIC。Core 的旧伴侣 listener 还把一个 QUIC stream 当作一个读到 EOF 的 JSON 请求，和 Android 已发布的 `ACQ1` 长连接分帧协议不一致。Go 服务也没有把实际监听地址和证书下发给伴侣，因此“ADB 兼容通道已连接、QUIC 未建立”会长期存在。

## 决策

1. Core 在独立的 `ADBCONTROL_COMPANION_LISTEN` UDP 地址监听伴侣连接，默认部署端口为 `45922/UDP`。
2. 远程客户端继续使用 `45921/UDP`；两类身份、ALPN、会话和状态不得混用。
3. Android 伴侣始终作为 QUIC 客户端主动连接 Core。ADB 只负责首次配置、证书轮换和故障恢复。
4. Core 通过本机 stdio `companion.connection.info` 只公开监听地址、TLS 名称、公钥证书 DER 和指纹，不公开私钥。
5. Go 根据显式 `WEBADB_COMPANION_PUBLIC_HOST` 或到无线设备的本机路由，生成手机实际可访问的 `quic://` 地址，再向显式广播接收器发送配置。
6. 控制 stream 固定使用 `ACQ1`、类型 `1` 和重复 `uint32_be + JSON` 帧；同一 stream 承载握手、心跳、状态同步、命令与响应。
7. Core registry 必须是网络线程和 stdio/远程线程共享的实时状态，不允许使用启动时快照。

## 原因

- Android 发起出站连接更适合 Wi-Fi 地址变化、后台重连和路由器客户端环境。
- ADB 不再是 QUIC 存活的前置条件，配置保存后即使无线调试关闭也可以重连。
- 独立端口可以避免远程管理用户协议和设备伴侣协议发生身份混淆。
- 精确证书固定避免依赖局域网 DNS 或公共 CA。

## 替代方案

- 由手机监听、Core 根据 ADB IP 主动拨号：拒绝。它依赖 ADB 持续在线，受 Android 后台监听、客户端隔离、多网卡和地址变化影响更大。
- 与远程管理共用 `45921/UDP`：拒绝。除非未来实现经过测试的 ALPN 分流，否则会扩大协议和权限边界。
- 在 Go 内实现第二套 QUIC 服务端：拒绝。设备协议及会话事实仍由 Rust Core 统一拥有。

## 影响范围

Core companion transport、实时 registry、stdio bootstrap IPC、Go 设备服务、Docker/发行版 UDP 端口和部署环境变量。

## 风险

- 容器或路由器必须映射并放行 `45922/UDP`。
- 多网卡设备需要显式设置 `WEBADB_COMPANION_PUBLIC_HOST`。
- 证书文件必须持久化；删除证书会要求通过 ADB 重新下发配置。
- 视频单向 stream 的消费与 Web 媒体网关属于后续独立阶段，不能阻塞控制会话。

## 未来演进

增加设备注册密钥或双向身份认证、连接阶段指标、视频 stream relay，以及在不改变安全边界的前提下评估 ALPN 端口复用。
