# 与 Node-RED 的比较

IoT Rule Engine 面向工业数据采集、连接资源治理和受控规则执行；Node-RED 面向广泛的事件驱动低代码集成。选择取决于现场要求和团队经验。

| 维度 | IoT Rule Engine | Node-RED |
| --- | --- | --- |
| 扩展生态 | 官方插件较少，固定 SDK 契约，独立制品 | 成熟的 Node.js/npm 和社区节点生态，快速原型优势明显 |
| 运行隔离 | Go 核心，插件为独立 OS 进程；同一产物内规则实例独立 | 通常在 Node.js Runtime 中运行节点，具体第三方节点也可自行使用外部进程 |
| 连接复用 | Connector 为一级资源，跨规则引用、探测、配额与生命周期统一管理 | configuration node 可共享配置和连接；MQTT In/Out 就使用这种模式 |
| PLC | 官方 S7 提供批读、串行门禁、按需求复用和熔断 | 具体行为由所选 PLC 节点实现，需要逐项验证 |
| 插件交付 | 每平台签名制品、摘要、SBOM、受控 Hub | Palette Manager/npm 提供安装和升级，生态更广泛 |
| 部署与使用 | 同一软件适配多边缘网关和一个集中服务器，管理 UI 随发行包交付 | 丰富的编辑、调试和可复用 flow/subflow 能力 |
| 代价 | 跨进程调用有开销；工业插件覆盖及社区成熟度仍需建设 | 通用灵活性较强；设备连接治理与工业可靠性取决于节点实现和部署约束 |

不能宣传“Node-RED 不支持连接共享”。本项目的比较重点是统一资源治理和经过验收的协议行为，也不能在没有同负载测试结果的情况下声称性能更高。

官方参考：[Configuration nodes](https://nodered.org/docs/creating-nodes/config-nodes)、[Palette Manager](https://nodered.org/docs/user-guide/editor/palette/manager)、[Flow structure](https://nodered.org/docs/developing-flows/flow-structure)。
