好的，以下是一个**完整的 Go 语言复刻版 Stowaway 多级代理工具的开发路线图**，我们将分阶段、按功能模块逐步构建，并优先实现核心通信和管理能力，最终达成稳定、可扩展、可跨平台的渗透测试辅助工具。

---

# 🛠️ Stowaway 复刻项目开发路线图

## ✅ 阶段一：核心架构与通信模型

**目标：实现 admin 与 agent 的基础连接 + 控制命令通道**

### ✔️ 子任务：

1. **项目结构设计**

   * `cmd/admin/`、`cmd/agent/`：入口程序
   * `core/network/`：连接管理（握手、封包、解包）
   * `core/protocol/`：定义消息类型和结构体（如 CONNECT、SHELL、UPLOAD）
   * `core/common/`：日志、配置、错误处理等工具包

2. **节点结构设计**

   * 每个节点维护：

     * 节点 ID
     * 父节点连接
     * 子节点列表
     * 状态（连接中、断开、重连中）
     * 备注（Memo）

3. **admin 与 agent 基本握手机制**

   * 主动连接/被动监听
   * 握手认证（密钥校验）
   * 节点注册流程
   * 支持 JSON 协议 or TLV 封包协议（推荐自定义结构体 + length-prefixed）

4. **基本命令协议封装**

   * 自定义控制包格式：`type | length | payload`
   * 支持简单命令下发与响应（ping、pong、echo、exit）

---

## ✅ 阶段二：多级节点管理与拓扑构建

**目标：构建 admin 控制台 + 支持多级代理链路**

### ✔️ 子任务：

1. **节点拓扑维护（树状结构）**

   * 每个节点可以连接/监听下游节点
   * admin 支持 topo 展示树形结构（基于 ID 或 ASCII 树）

2. **admin 控制台实现**

   * REPL 输入循环（支持命令补全）
   * 支持 `use <id>`、`detail`、`topo`、`connect`、`listen` 等基础指令

3. **节点命令分发与路由**

   * admin -> agent0 -> agent1 -> agent2 -> ...
   * 控制命令逐层传递（包中带目标节点路径）

---

## ✅ 阶段三：交互式 shell 与文件传输

**目标：支持远程 shell、上传下载功能**

### ✔️ 子任务：

1. **远程 shell（exec 模式）**

   * agent 启动一个 `bash`/`sh` 子进程，绑定 stdin/stdout
   * 利用数据通道实现实时输入输出传输（基于多路复用）
   * 可选支持 creack/pty（完整伪终端支持）

2. **文件上传与下载**

   * 支持单文件传输命令：`upload local remote` / `download remote local`
   * 流控机制（分片、确认、完整性校验）
   * 支持显示传输进度

---

## ✅ 阶段四：端口转发、socks5、反向连接

**目标：实现代理功能与正/反向端口映射**

### ✔️ 子任务：

1. **socks5 代理服务**

   * 在 admin 上启动 socks5 server，转发流量至某个 agent 通道
   * 基于 agent 构建链路，转发请求

2. **端口转发与反向映射**

   * forward：admin -> agent -> target\:port
   * backward：agent 监听端口，将流量反向转发至 admin 本地端口

3. **代理链路中的流量中转设计**

   * TCP 数据转发中继机制
   * 支持链路连接检查、连接断开自动清理

---

## ✅ 阶段五：高级功能与平台适配

**目标：提升稳定性与跨平台兼容性**

### ✔️ 子任务：

1. **TLS / AES-256-GCM 加密**

   * TLS 双向验证（admin/agent 证书）
   * AES 对称密钥加密（支持密钥协商或预共享）

2. **重连机制**

   * agent 主动模式下 `--reconnect` 间隔重连
   * 被动模式 agent 自动恢复监听
   * admin 可尝试恢复节点链路

3. **端口复用机制**

   * 基于 SO\_REUSEPORT / SO\_REUSEADDR（Win/mac/Linux）
   * 基于 IPTABLES（Linux）+ 脚本工具自动配置清理

4. **平台适配与交叉编译**

   * Makefile 支持多平台编译：Windows、Linux、MIPS、ARM

---

## ✅ 阶段六：交互优化与命令集完整化

**目标：还原原版控制台全部功能 + 美化交互**

### ✔️ 子任务：

1. 控制台支持：

   * tab 补全（使用 `github.com/c-bata/go-prompt`）
   * 命令历史、上下切换
   * 交互输出美化（颜色、高亮、ASCII 树）

2. 命令集完整复刻：

   * `upload/download`
   * `socks/stopsocks`
   * `forward/backward/stopforward/stopbackward`
   * `memo/addmemo/delmemo`
   * `shutdown/back/exit`
   * `ssh/sshtunnel`
   * `listen` 三种模式支持（普通、SOReuse、IPTABLES）

---

## 🎯 阶段七：稳定性、安全性与发布

**目标：项目收尾与测试发布**

### ✔️ 子任务：

1. 完善日志系统（agent 可输出标准日志 / admin 实时监控）
2. 编写配置文件支持（YAML/JSON），用于批量部署
3. 编写项目 README、使用手册、快速部署文档
4. 编写测试脚本与网络环境模拟器
5. Release 自动构建产物（GitHub Actions 或 Makefile）

---

## 📌 项目分工建议（如多人合作）

| 角色      | 负责模块                  |
| ------- | --------------------- |
| 通信协议设计者 | 定义消息协议、封包/解包、安全策略     |
| 网络开发者   | TCP/WS通信、shell实现、文件传输 |
| 控制台开发者  | REPL控制台、命令行 UI、美化交互   |
| 安全策略开发者 | TLS、AES、节点验证、加密握手流程   |
| 跨平台构建者  | Makefile、多平台编译、端口复用支持 |
| 文档维护者   | 使用说明、示例脚本、交付文档        |

---

# Relay机制设计


---

## ✅ 接受并整合你的两个提议

### ✅ 1）**仍采用你提出的 ID 分配方法**

* 每个 relay 节点向 admin 注册后，admin 为其 **分配一段编号空间**（例如 1000 \~ 1999）；
* relay 将这段编号用于为其 **所有后代节点**分配 ID（避免冲突）；
* 每个 relay **自行管理该编号段的分配状态**。

---

### ✅ 2）**每个 relay 拥有独立的逻辑视图**

* 每个 relay 都有自己的：

  * `Registry`：子节点连接与 ID 的本地管理；
  * `NodeGraph`：仅记录以自己为 root 的局部拓扑结构；
* admin 作为全局视图的根拥有全图；
* 中继 relay 只需要关心**自己以下的子图**；
* 消息发送基于 **本地注册表+路由表**决定是否下发或上发。

---

## 🧭 全流程设计：中继注册与转发机制（v3）

---

### 🟢 **启动阶段**

#### 1️⃣ Admin 监听 agent 连接（现有逻辑）

#### 2️⃣ Admin 控制 agentX 成为 relay

* 发送 `MsgStartRelay`：

```go
type StartRelayPayload struct {
	ListenAddr string
	IDStart    int
	Count      int // 分配空间数量，如 1000
}
```

* agentX 记录其编号空间 `IDPool = [IDStart, IDStart+Count-1]`

---

### 🟢 **relay 启动监听器并创建本地结构**

agentX 调用：

```go
StartRelayListener(addr string, selfID int, registry *node.Registry, topo *node.NodeGraph, idRange IDAllocator)
```

> `IDAllocator` 维护分配状态。

---

### 🟢 **新的 agentY 连接 relayX**

relayX：

1. 读取 handshake；
2. 为其分配一个可用 ID：Y ∈ IDPool；
3. 构建 `node.Node{ID: Y}`；
4. 注册进本地 `registry` 与 `topo.SetParent(Y, X)`；
5. 使用 `MsgRelayRegister` 报告 admin：

```go
type RelayRegisterPayload struct {
	ParentID int
	Node     node.Node
}
```

---

### 🟢 **Admin 注册新节点 Y**

* admin 使用 `registry.Add()`、`topo.SetParent(Y, X)`；
* 回复 relayX `MsgRelayAck{Success: true}`。

---

### 🟢 **后续转发全部通过统一 RelayPacket**

```go
type RelayPacket struct {
	DestID int
	Data   []byte // 嵌套 Message
}
```

中继 relay 路由逻辑：

* 若 `DestID ∈ 本地 registry`：直发；
* 若不是：找本地 topo 中哪个子 relay 拥有该节点 → 转发；
* 找不到：返回错误。

---

## ✅ Relay 本地结构布局建议

```go
type RelayContext struct {
	SelfID     int
	Registry   *node.Registry
	Topology   *node.NodeGraph
	IDAllocator *IDPoolAllocator
	Upstream    net.Conn // 与 admin 的连接
}
```

---

## 📊 逻辑关系图（结构树）

```
Admin:
  Registry: [1000, 1001, 1002, ...]
  Topology:
    1000
    ├── 1001
    └── 1002

Relay (ID=1000):
  Registry: [1001, 1002]
  Topology:
    1000
    ├── 1001
    └── 1002
```

---

## ✅ 接下来可进入的编码工作子任务：

| 子任务                                                            | 描述                                                         |
| -------------------------------------------------------------- | ---------------------------------------------------------- |
| 🧱 定义协议结构体                                                     | `StartRelayPayload`、`RelayRegisterPayload`、`RelayPacket` 等 |
| 🔧 编写 relay ID 分配器                                             | 简单的 `IDAllocator`，支持 get/free                              |
| 🚀 实现 `StartRelayListener()`                                   | 支持本地结构注册，回报 admin                                          |
| 🔁 实现 `HandleRelayConnection()` 和 `HandleRelayAgentMessages()` | 接入与处理                                                      |
| ↕️ 实现 `HandleRelayPacket()`                                    | 透明转发                                                       |
| 📤 上报逻辑                                                        | relay 向 admin 报告新节点                                        |
| 📥 admin handler                                               | 处理 relay 注册消息、构建全局拓扑                                       |

---

你是否希望我 **从结构体 + MessageType 开始逐个生成完整代码**，还是你希望我先出整体目录结构或先实现某一部分（如 ID allocator）？我们可以按模块节奏推进。
