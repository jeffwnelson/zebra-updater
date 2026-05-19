package printer

import (
	"bufio"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
)

// fakeServer accepts a single connection, reads SGD getvar lines, and
// replies with values from a map. Unknown vars get an empty quoted string.
type fakeServer struct {
	t       *testing.T
	listen  net.Listener
	addr    string
	vars    map[string]string
	mu      sync.Mutex
	dropMid bool // if true, close the connection after one response
}

func newFakeServer(t *testing.T, vars map[string]string) *fakeServer {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	f := &fakeServer{t: t, listen: l, addr: l.Addr().String(), vars: vars}
	t.Cleanup(func() { l.Close() })
	go f.serve()
	return f
}

func (f *fakeServer) host() string {
	host, _, _ := net.SplitHostPort(f.addr)
	return host
}

func (f *fakeServer) port() int {
	_, p, _ := net.SplitHostPort(f.addr)
	var n int
	for _, c := range p {
		n = n*10 + int(c-'0')
	}
	return n
}

func (f *fakeServer) serve() {
	for {
		conn, err := f.listen.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakeServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	served := 0
	for {
		line, err := r.ReadString('\n')
		if err == io.EOF {
			return
		}
		if err != nil {
			return
		}
		name := extractGetVarName(line)
		f.mu.Lock()
		val, ok := f.vars[name]
		drop := f.dropMid
		f.mu.Unlock()
		if !ok {
			val = ""
		}
		conn.Write([]byte("\"" + val + "\"\r\n"))
		served++
		if drop && served == 1 {
			return
		}
	}
}

func extractGetVarName(line string) string {
	// "! U1 getvar \"name\"\r\n" → "name"
	i := strings.Index(line, "\"")
	j := strings.LastIndex(line, "\"")
	if i < 0 || j <= i {
		return ""
	}
	return line[i+1 : j]
}
