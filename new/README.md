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


---

📄 **推荐的 Git 提交信息格式如下：**

```git
feat(agent): refactor agent message handling and restore persistent shell via pty

- Move agent logic to core/network/dispatcher.go
- Support MsgCommand and MsgShell via shared PTY
- Enable interactive shell session with context preservation (cd, export, etc.)
```

说明：

* `feat(...)` 用于功能性变更
* 第一行是简短摘要（<50字符），第二行空行，之后是详细说明

---

## ✅ 2. **Git commit 常见 message 格式（Conventional Commits）**

这是社区通用规范 [Conventional Commits](https://www.conventionalcommits.org/) 的简写模板：

```txt
<type>(<scope>): <short summary>
<BLANK LINE>
<details (wrap at 72 chars)>
```

常见 `<type>` 类型：

| 类型         | 含义                 |
| ---------- | ------------------ |
| `feat`     | 新功能                |
| `fix`      | Bug 修复             |
| `refactor` | 重构（非新功能/修复）        |
| `chore`    | 构建工具、依赖等变更         |
| `test`     | 添加或重构测试            |
| `docs`     | 文档变更               |
| `style`    | 格式调整，不影响语义（如空格、缩进） |
| `perf`     | 性能优化               |

示例：

```txt
feat(protocol): support MsgShell with interactive PTY

Enable remote shell with creack/pty. Supports cd, export, etc.
```

---

## ✅ 3. **五个后续功能的优先级推荐（建议实现顺序）**

| 优先级 | 模块                       | 理由                      |
| --- | ------------------------ | ----------------------- |
| ✅ 1 | **upload/download 文件传输** | 基础功能，使用频率高，逻辑较清晰易实现     |
| ✅ 2 | **拓扑连接：listen/connect**  | 实现多级代理所必需，为树状结构提供基础     |
| ✅ 3 | **socks5 转发**            | 实现代理效果（例如访问内网），依赖已有连接结构 |
| ✅ 4 | **TLS 加密与认证机制**          | 网络安全增强，建议拓扑结构稳定后实现      |
| ✅ 5 | **日志模块化与 debug 面板**      | 用于稳定性调试与监控，适合中后期开发阶段    |

---

## ✅ 总结

* ✍️ 本次提交推荐用 `feat(agent): ...` + 多行说明。
* 📌 Conventional Commits 是结构化 commit 的主流规范。
* 🔧 建议优先实现：

  1. `upload/download`
  2. `listen/connect`
  3. `socks5`
  4. `TLS`
  5. `debug`

是否现在开始实现文件上传（`upload`）功能？我可以帮你定义协议、admin/agent 指令、传输缓冲结构。
