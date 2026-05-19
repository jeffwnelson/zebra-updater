package printer

import (
	"bufio"
	"fmt"
	"net"
	"time"
)

type Printer struct {
	Host    string
	Port    int
	Timeout time.Duration
}

func (p *Printer) address() string {
	return fmt.Sprintf("%s:%d", p.Host, p.Port)
}

// dial opens a TCP connection with the configured timeout.
func (p *Printer) dial() (net.Conn, error) {
	d := net.Dialer{Timeout: p.Timeout}
	return d.Dial("tcp", p.address())
}

// GetVar performs one SGD getvar query, opening and closing a connection.
func (p *Printer) GetVar(name string) (string, error) {
	conn, err := p.dial()
	if err != nil {
		return "", fmt.Errorf("connect %s: %w", p.address(), err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(p.Timeout))

	if _, err := conn.Write([]byte(BuildGetVar(name))); err != nil {
		return "", fmt.Errorf("send getvar: %w", err)
	}
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	return ParseResponse(line)
}
