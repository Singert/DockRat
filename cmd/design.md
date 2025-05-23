好问题。我们来**精确区分**这两种方式对“持久化 shell session”的支持情况。

---

## ✅ 你现在有两种远程 shell 模式：

| 模式类型                     | 描述                                | 是否持久化 | 用途                      |
| ------------------------ | --------------------------------- | ----- | ----------------------- |
| **结构化交互 (`exec`)**       | 后台维护一个 shell 实例，每条命令发送后执行         | ✅ 是   | 自动化、一次一条命令、后台执行         |
| **完整 pty shell (`cmd`)** | 启动一个 bash，开启 pty 终端双向流 io.Copy 模式 | ✅ 是   | 模拟 ssh，手动交互、cd、历史、提示符保留 |

---

## ✅ 所以答案是：**是的，仍然持久化，而且更真实**

你调用的是：

```go
cmd := exec.Command("bash")
ptmx, _ := pty.Start(cmd)
```

只要：

* 这个 `cmd` 没有退出
* 你没关闭 `ptmx`

它就**持续存在**，你在其中执行的 `cd`、`export`、`alias`、函数定义等都 **保留上下文**。

---

## ✅ 举例证明：

```bash
Admin> shell
$ cd /tmp
$ touch x
$ ls
x
$ export TEST=123
$ echo $TEST
123
```

你退出 shell（`exit`）才会关闭它。

---

## 🧠 总结：

| 特性               | `exec` 模式 | `shell` 模式（pty） |
| ---------------- | --------- | --------------- |
| 持久上下文（cd/export） | ✅ 是       | ✅ 是             |
| 输出结构化            | ✅ 是       | ❌（原始终端流）        |
| 可脚本化             | ✅ 适合      | ❌ 不适合           |
| 模拟终端             | ❌ 否       | ✅ 完整 bash       |
| 保留提示符/补全         | ❌ 否       | ✅ 有 bash 行为     |

---

你完全可以双模共存：

* 自动命令：用 `exec`
* 手动控制：用 `shell`

---

需要我为你建立一个状态机，支持多节点间切换 shell 会话（如 Metasploit 的 `sessions -i 1`）吗？或者现在加上 `upload`？
