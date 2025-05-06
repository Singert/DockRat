// cmd/main.go
package main

// import (
// 	"fmt"
// 	"net"
// 	"os"

// 	"github.com/Singert/DockRat/core/node"
// 	"github.com/Singert/DockRat/core/protocol"
// )

// func main() {
// 	if len(os.Args) < 2 {
// 		fmt.Println(fmt.Println("Usage: stowaway <admin|agent>"))
// 		return
// 	}
// 	role := os.Args[1]

// 	switch role {
// 	case "admin":
// 		startAdmin()
// 	case "agent":
// 		startAgent()
// 	default:
// 		fmt.Println("unknown role")
// 	}
// }

// func startAdmin() {
// 	fmt.Println("Starting Admin")

// 	ln, err := net.Listen("tcp", ":9999")
// 	if err != nil {
// 		fmt.Println("error listening:", err)
// 		return
// 	}
// 	fmt.Println("admin is listening on")
// 	nodeManager := node.NewNoddManager()
// 	for {
// 		conn, err := ln.Accept()
// 		if err != nil {
// 			fmt.Println("error accept connection: ", err)
// 			continue
// 		}
// 		nodeID := nodeManager.AddNode(conn)
// 		fmt.Println("New agent connected with ID:", nodeID)
// 		go protocol.HandleConn(conn, true, nodeID)
// 	}

// }

// func startAgent() {
// 	conn, err := net.Dial("tcp", "127.0.0.1:9999")
// 	if err != nil {
// 		fmt.Println("error connecting to admin:", err)
// 		return
// 	}
// 	fmt.Println("Connected to admin")
// 	protocol.HandleConn(conn, false, -1)
// }
