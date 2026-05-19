package printer

import (
	"bufio"
	"fmt"
	"io"
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

// Ping verifies the printer accepts a TCP connection on the configured port.
func (p *Printer) Ping() error {
	conn, err := p.dial()
	if err != nil {
		return err
	}
	return conn.Close()
}

// Send streams data to the printer over a single TCP connection.
// progress is invoked periodically with the cumulative bytes written.
// progress may be nil.
func (p *Printer) Send(r io.Reader, progress func(written int64)) error {
	conn, err := p.dial()
	if err != nil {
		return fmt.Errorf("connect %s: %w", p.address(), err)
	}
	defer conn.Close()

	buf := make([]byte, 32*1024)
	var total int64
	for {
		n, readErr := r.Read(buf)
		if n > 0 {
			if err := conn.SetWriteDeadline(time.Now().Add(p.Timeout)); err != nil {
				return fmt.Errorf("set write deadline: %w", err)
			}
			if _, werr := conn.Write(buf[:n]); werr != nil {
				return fmt.Errorf("write to printer: %w", werr)
			}
			total += int64(n)
			if progress != nil {
				progress(total)
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return fmt.Errorf("read firmware: %w", readErr)
		}
	}
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
