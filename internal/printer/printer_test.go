package printer

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestGetVar(t *testing.T) {
	fake := newFakeServer(t, map[string]string{
		"device.product.name": "ZD621-203dpi",
	})
	p := &Printer{Host: fake.host(), Port: fake.port(), Timeout: 2 * time.Second}
	got, err := p.GetVar("device.product.name")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	if got != "ZD621-203dpi" {
		t.Errorf("got %q, want ZD621-203dpi", got)
	}
}

func TestGetVarUnknownReturnsEmpty(t *testing.T) {
	fake := newFakeServer(t, map[string]string{})
	p := &Printer{Host: fake.host(), Port: fake.port(), Timeout: 2 * time.Second}
	got, err := p.GetVar("nonexistent.var")
	if err != nil {
		t.Fatalf("GetVar: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty string", got)
	}
}

func TestGetVarConnectFails(t *testing.T) {
	p := &Printer{Host: "127.0.0.1", Port: 1, Timeout: 500 * time.Millisecond}
	if _, err := p.GetVar("any.var"); err == nil {
		t.Fatal("expected connect error, got nil")
	}
}

func TestPing(t *testing.T) {
	fake := newFakeServer(t, map[string]string{})
	p := &Printer{Host: fake.host(), Port: fake.port(), Timeout: time.Second}
	if err := p.Ping(); err != nil {
		t.Errorf("Ping: %v", err)
	}
}

func TestPingUnreachable(t *testing.T) {
	p := &Printer{Host: "127.0.0.1", Port: 1, Timeout: 200 * time.Millisecond}
	if err := p.Ping(); err == nil {
		t.Error("expected error, got nil")
	}
}

func TestSendStreamsAllBytes(t *testing.T) {
	// Use a raw TCP listener (not the SGD fake) and record everything received.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	received := make(chan []byte, 1)
	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := new(bytes.Buffer)
		buf.ReadFrom(conn)
		received <- buf.Bytes()
	}()

	host, port := splitHostPortForTest(t, l.Addr().String())
	p := &Printer{Host: host, Port: port, Timeout: 2 * time.Second}

	payload := bytes.Repeat([]byte("FIRMWARE"), 1024) // 8 KiB
	var progressCalls int
	err = p.Send(bytes.NewReader(payload), func(written int64) { progressCalls++ })
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	got := <-received
	if !bytes.Equal(got, payload) {
		t.Errorf("server received %d bytes, want %d", len(got), len(payload))
	}
	if progressCalls == 0 {
		t.Error("progress callback never invoked")
	}
}

func splitHostPortForTest(t *testing.T, addr string) (string, int) {
	t.Helper()
	host, p, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatal(err)
	}
	var port int
	for _, c := range p {
		port = port*10 + int(c-'0')
	}
	return host, port
}
