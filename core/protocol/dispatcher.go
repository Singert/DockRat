// === core/protocol/protocol.go ===

package protocol

import (
	"bufio"
	"fmt"
	"net"
	"strings"
)

type HandlerFunc func(msg Message) error

type Dispatcher struct {
	handlers map[string]HandlerFunc
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{
		handlers: make(map[string]HandlerFunc),
	}
}

func (d *Dispatcher) Register(msgType string, handler HandlerFunc) {
	d.handlers[msgType] = handler
}

func (d *Dispatcher) Dispatch(msg Message) error {
	h, ok := d.handlers[msg.Type]
	if !ok {
		return fmt.Errorf("unhandled message type: %s", msg.Type)
	}
	return h(msg)
}

func (d *Dispatcher) Listen(conn net.Conn) error {
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		msg, err := Decode([]byte(line))
		if err != nil {
			return fmt.Errorf("error decoding message: %v", err)
		}
		if err := d.Dispatch(msg); err != nil {
			return fmt.Errorf("error dispatching message: %v", err)
		}
	}
	return scanner.Err()
}

func (d *Dispatcher) ListenOnce(conn net.Conn) error {
	scanner := bufio.NewScanner(conn)
	if scanner.Scan() {
		msg, err := Decode(scanner.Bytes())
		fmt.Printf("[*] Received message:%+v\n", msg)
		if err != nil {
			return err
		}
		if handler, ok := d.handlers[msg.Type]; ok {
			return handler(msg)
		}
		return fmt.Errorf("unhandled message type: %s", msg.Type)
	}
	return scanner.Err()
}
