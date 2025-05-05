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

