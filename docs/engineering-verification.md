# 多网关工程化实施与验收

## 部署模型

测试工作区为 `/home/sun/punkiot-work`，各仓库在 `src/` 下独立克隆，
运行数据在 `runs/` 下按测试批次隔离。原开发目录不作测试数据目录。
不依赖 go.work 或本地 replace，SDK 公共契约保持 v1.0.0。

一个工厂可配置多个 Linux AMD64/ARM64 采集网关，每台运行独立 Engine、UI、
SQLite、内置 NATS 和插件进程；服务器运行一个 Engine，使用多规则汇聚。
设备由唯一网关负责。Connector 在本机跨规则复用，不跨网关共享会话。

## 实际链路验证

在用户授权的外部 EMQX 上使用随机隔离主题，启动三个独立网关 Engine 和一个
服务器 Engine。每个网关运行两条模拟采集规则及一条模拟下发规则。
实际使用 MQTT Connector、MQTT Source/Sink、JavaScript Transform、持久 buffer-node、
插件进程宿主和 Manager API。业务入库和 PLC IO 使用测试专用 SQLite/模拟插件。

已验证：

- 单网关 Broker 网络中断，其他网关继续上报。
- 每个设备 60 条、共 360 条唯一遥测，断网网关强制结束并重启后全部补传入库。
- 下发网关隔离，重复、过期、错误目标的模拟写入不会重复执行。
- 同一 Engine 两条采集规则共用一个 buffer 插件进程和一个 MQTT Connector 进程。
- 取消部署的关闭顺序保留节点结束报告和资源清理权限，避免重试等待。
- 下游先启动，避免 Source 或缓冲重放在下游就绪前丢失首条事件。
- 可靠模式贯通配置到执行器，并在节点完成后确认规则边。

运行入口：`test/e2e/multi_gateway.py`。Broker URL、用户名、密码只从环境注入，
不在仓库写入真实地址或凭证。测试不触碰现场 PLC，不向生产业务主题发布。

## 交付检查

Engine/UI 同源嵌入发行程序。Linux 安装脚本识别平台、验证固定发布者的签名、
配置 systemd、保留部署参数、备份升级数据库，并对健康失败执行恢复。
插件按 ID/版本/平台打包，SDK 版本独立；Hub 与 Engine 分别检查签名、摘要、
平台及握手，运行中的引用阻止替换。签名旧版本本地保留供回滚。

测试结果以本次实际执行记录及 CI 状态为准。原生 ARM64 runner 的构建与测试，
不能由 AMD64 交叉编译结果替代。FANUC、三菱完整协议插件和 Fleet 仍为规划。

## 已发布验收记录

- Engine v1.1.3：AMD64 与 ARM64 原生 runner 完成构建、签名和空目录启动；
  [发行流程](https://github.com/punk-one/iot-rule-engine/actions/runs/43381134332)。
- 插件集合 v1.0.3：仅选择 9 个插件，按各自版本和平台单独打包；
  [原生双架构验收](https://github.com/punk-one/iot-rule-plugins/actions/runs/43372941296)。
- 插件集合 v1.0.4：仅发布 connect-mqtt v1.0.2，修复资源删除与建连竞态；
  [单插件双架构验收](https://github.com/punk-one/iot-rule-plugins/actions/runs/43374835702)。
- 本地前端 20 个测试套件、59 项测试及生产构建通过；Engine 全量测试、竞态检查、
  go vet、官方插件独立模块验收与 SDK v1 契约测试通过。
- 多网关链路在 MQTT 资源关闭修复后再次通过：360 条唯一遥测完成断网、强制重启补传。
- 真实 Hub 的平台筛选、单插件签名安装、规则引用保护、升级、已验证版本回滚、
  回滚后的重启选定均通过；Connector 存在物理资源租约时拒绝替换，停用无绑定的
  资源会释放运行时对象，保留配置后可继续回滚。升级后 Provider 会刷新到新进程。
- 安装器使用实际签名发行包通过安装与 UI/API 启动检查；测试下载经已核对的本地缓存，
  不修改系统服务或防火墙。原生双架构 CI 验证实际可执行文件。
- Hub 部署对 ELF 二进制响应启用 gzip，保持解压后的 SHA-256 和签名校验。
  JavaScript 插件约 19 MB 原文件压缩传输约 7.1 MB，实测约 16 秒。

## 验证边界

不把 QoS 1 的 Broker 接收确认解释成数据库提交，不承诺 exactly once。
服务器数据库必须以 event_id 幂等。服务器离线但 Broker 在线的完整补偿还需要
持久订阅验证或应用回执；不包含在此次断网网关验证中。
实际 PLC 的读写耗时、100ms 采集能力、物理写入幂等和生产时序库应单独现场验收。
