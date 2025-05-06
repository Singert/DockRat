# DockRat
课程作业-对项目Stowaway的仿写
复刻 **Stowaway** 项目涉及多个方面，包括多级网络通信、命令调度、节点管理、TLS/加密通信、文件传输等功能，整个体系设计复杂但逻辑清晰。以下是复刻该项目的整体技术路线和模块建议：

---

## 🧩 核心功能模块划分

### 1. **基础通信层（Core Transport）**

* 支持 TCP/HTTP/WebSocket 三种协议通信（裸TCP、HTTP消息封装、WebSocket长连接）
* 支持主动连接（Client）和被动监听（Server）模式
* 支持 Socks5/HTTP 代理穿透
* 可选 TLS 加密（建议使用 `crypto/tls` 标准库）
* 实现通信心跳机制，保持连接活性

### 2. **加密模块（Crypto Layer）**

* 使用对称加密（如 AES-256-GCM）加密节点间传输的数据流
* 密钥交换基于预共享密钥（可选 Diffie-Hellman 改进）
* TLS 和 AES 可独立控制（使用 TLS 时禁用 AES）

### 3. **节点管理模块（Node Tree）**

* 构造基于树的网络结构（每个节点有父节点和多个子节点）
* 管理每个节点的状态（上线/掉线）、代理服务（socks/forward/backward）、备注等信息
* 多节点间通过 ID 或编号识别，保持拓扑图实时更新

### 4. **命令控制协议（Command Protocol）**

* 定义统一的指令格式（JSON或自定义协议）
* 典型指令：connect、listen、ssh、shell、forward、backward、upload、download、shutdown 等
* 建议使用状态机来解析处理命令（支持 command dispatch）

### 5. **管理端界面（Admin CLI）**

* 设计一个交互式 CLI（带命令历史、补全、导航）
* 状态展示如 `detail` / `topo`
* 节点 `use` 后支持子命令（节点服务管理）

### 6. **文件传输模块**

* 自定义简易传输协议（头信息+长度+数据块）
* 断点续传、上传下载文件，带进度显示

### 7. **端口转发模块**

* `forward`: Admin监听本地端口，将流量转发给远程节点的目标端口
* `backward`: Agent监听本地端口，反向将流量转发回Admin端口
* 支持多连接、连接管理

### 8. **端口复用模块（高级功能）**

* Linux：支持 `iptables` 重定向
* Windows/Mac：使用 `SO_REUSEPORT` 和 `SO_REUSEADDR`
* 提供脚本和 agent 配置交互，确保复用前后服务正常

---

## 🛠️ 技术选型建议（Go 实现）

| 功能     | 推荐包/工具                                                         |
| ------ | -------------------------------------------------------------- |
| 网络通信   | `net`, `net/http`, `golang.org/x/net/websocket`                |
| TLS 加密 | `crypto/tls`                                                   |
| AES 加密 | `crypto/aes`, `crypto/cipher`                                  |
| 命令行交互  | `github.com/c-bata/go-prompt`, `github.com/AlecAivazis/survey` |
| 并发处理   | `goroutines`, `channels`, `sync.Map`                           |
| 日志记录   | `log`, `github.com/sirupsen/logrus`                            |
| 跨平台构建  | `Makefile`, `go build` with `GOOS` / `GOARCH`                  |
| 多平台支持  | `build constraints`, 编译时用 `GOARCH=arm/mips` 等                  |
| 文件传输   | 自定义流式协议或基于 `io.Copy` + `bufio`                                 |

---

## 🧱 项目结构建议

```bash
stowaway-clone/
├── cmd/
│   ├── stowaway_admin/
│   └── stowaway_agent/
├── core/
│   ├── transport/         # 传输层（TCP/HTTP/WS）
│   ├── crypto/            # 加密模块（AES/TLS）
│   ├── node/              # 节点管理
│   ├── protocol/          # 命令协议定义
│   ├── filetransfer/      # 文件上传/下载
│   ├── tunnel/            # forward/backward/ssh隧道
├── pkg/
│   └── utils/
├── script/                # reuse.py、辅助脚本
├── Makefile
└── README.md
```

---

## 📌 开发建议

1. **优先实现最小可用原型**：

   * Admin + 单个 Agent（TCP 通信，shell 功能）
   * 再扩展：Socks5, 文件传输, TLS, 多级连接

2. **使用 protobuf/json 定义消息结构体**，标准化传输内容

3. **设置统一的日志调试开关**，便于排查节点间通信问题

4. **复刻 listen/connect/sshtunnel 命令功能**时注意 CLI 状态管理（面板切换）

5. **采用 interface 封装传输抽象层**，比如 `ConnWrapper`，适配 TCP/TLS/WS等不同类型连接

---

## ✅ 最终目标功能清单（对照原版）

| 功能               | 实现建议                       |
| ---------------- | -------------------------- |
| 多级节点拓扑           | 树结构 + ID映射                 |
| TCP/HTTP/WS流量支持  | 使用统一流量封装层                  |
| TLS/AES-GCM加密    | `tls.Conn` / `cipher.AEAD` |
| 远程 shell / ssh隧道 | `os/exec` + pty            |
| 文件上传下载           | `io.Copy` 实现分块传输           |
| socks5/http代理    | `golang.org/x/net/proxy`   |
| 端口正向/反向映射        | 建立监听端口+多协程                 |
| 跨平台编译支持          | `Makefile + go build`      |
| CLI界面（带补全）       | `go-prompt`                |
| 端口复用/IPTABLES整合  | 提供外部 Python 脚本辅助           |

---

如你希望我直接生成一个 **初始框架项目** 或提供部分代码模块（如 TLS 通信、节点管理、传输封装、shell 执行），我可以为你生成。你希望我先从哪个模块开始？

当然可以，我们可以将**复刻 Stowaway 的开发过程分为五个阶段**，每个阶段都明确目标、重点模块和建议技术栈。你只需按部就班，一步步来，项目就能顺利推进。

---

## 🧭 阶段化开发路线

### ✅ **阶段一：基础通信层与最小可用原型 (MVP)**

> **目标**：实现 admin 与 agent 之间的 TCP 通信和命令交互，最简单的远程命令执行（shell）

#### ✔ 你要实现的功能：

* `admin`：监听端口，等待连接
* `agent`：主动连接 admin
* 双向 TCP 通信：基于自定义协议传输命令和结果
* 执行远程命令（用 `bash -c`）并返回结果

#### 📁 模块目录建议：

```bash
core/
  ├── protocol/       # 协议定义（结构体 + 编码/解码）
  ├── shell/          # 执行命令并返回输出
cmd/
  ├── stowaway_admin/
  └── stowaway_agent/
```

#### 🧱 技术点建议：

* `net.Conn` 基础通信（TCP）
* `encoding/gob` 或 `encoding/json` 实现简单的消息结构封装
* `bufio.Reader/Writer` 实现数据读写
* `os/exec` 运行命令并收集输出

---

### ✅ **阶段二：多级节点与命令系统设计**

> **目标**：agent 可以作为中转节点，连接新的 agent，形成**树形结构**

#### ✔ 你要实现的功能：

* 每个节点可以转发数据给它的父节点或子节点
* `use` 命令切换控制的节点
* `connect` 命令让一个节点连接另一个 agent

#### 📁 新增模块：

```bash
core/
  ├── node/           # 节点结构与树管理
  ├── router/         # 节点路由与转发
```

#### 🧠 技术点：

* 节点分配 ID，维护父子结构
* 每个连接都要有 metadata：ID、父子关系、状态
* 路由模块实现从 admin -> node1 -> node2 -> node3 的链式消息转发

---

### ✅ **阶段三：Socks5、Shell、File 等服务功能**

> **目标**：实现 socks5 代理、远程 shell、上传下载等关键服务

#### ✔ 你要实现的功能：

* 远程命令执行交互模式
* 文件上传/下载（可用分块流式传输）
* 本地启动 socks5，转发给 agent 出口访问

#### 🧠 技术点：

* `socks5`: 用现成的 [xtaci/smux](https://github.com/xtaci/smux) 或自己实现简单版本
* shell：利用 pty 提供交互式 shell
* 文件：`io.Copy` + metadata 描述传输流

---

### ✅ **阶段四：加密与协议切换支持**

> **目标**：通信支持 TLS、AES-GCM、HTTP/WS 模式切换

#### ✔ 你要实现的功能：

* TLS 握手（双向）
* AES-GCM 加密解密
* WS/HTTP 伪装为应用层流量（套在 TCP 上）

#### 🧠 技术点：

* TLS：`crypto/tls`
* AES：`crypto/aes`, `crypto/cipher`
* WebSocket：`golang.org/x/net/websocket` 或 `nhooyr.io/websocket`

---

### ✅ **阶段五：CLI交互界面与完整控制面板**

> **目标**：实现 admin 的完整命令行操作界面（支持补全、历史记录）

#### ✔ 你要实现的功能：

* 命令面板 (`help`, `use`, `listen`, `ssh`, `connect`, `upload`, `forward`, …)
* 面板切换（admin主界面 <=> node控制面板）
* 状态跟踪、进度条显示、错误反馈

#### 推荐使用的库：

* [c-bata/go-prompt](https://github.com/c-bata/go-prompt)：提供高级命令行交互
* 或者自己实现：用 `bufio.Scanner` 处理命令输入 + 手动 dispatch 命令

---

## 🔨 建议你先做的第一步（立即可做）

创建最小可用原型，实现最简单的 admin/agent TCP 通信 + 远程执行命令：

```
admin: ./stowaway_admin -l 9999
agent: ./stowaway_agent -c 127.0.0.1:9999
```

1. 我可以直接给你写出这个部分的完整代码结构
2. 然后你就可以在终端里看到：admin 发一条命令，agent 执行后返回结果 ✅

---

是否现在就从「**阶段一：admin 和 agent 的远程命令执行**」这个最小原型开始？我可以立刻给你生成对应的代码框架。你希望先实现哪种：命令单发返回，还是持续交互式 shell？
