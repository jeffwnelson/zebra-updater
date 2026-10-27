package zebra

import (
	"bufio"
	"bytes"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// zd621Vars mirrors what a real ZD621 on Link-OS 6.3 answers.
var zd621Vars = map[string]string{
	"device.product_name":             "ZD621",
	"appl.name":                       "V93.21.07Z",
	"appl.link_os_version":            "6.3",
	"device.unique_id":                "D7J000000000",
	"device.allow_firmware_downloads": "yes",
	"device.host_status":              "030,0,0,1809,000,0,0,0,000,0,0,0\r\n000,0,0,0,0,2,6,0,00000000,1,000\r\n0000,0",
}

// fakePrinter speaks enough Link-OS to exercise this package: it answers
// getvar like the real firmware (quoted, no trailing newline, "?" for
// unknown variables), and when a connection starts with "! PROGRAM" it
// swallows the image, goes offline, and comes back on newVersion.
type fakePrinter struct {
	t    *testing.T
	addr string

	mu         sync.Mutex
	l          net.Listener
	vars       map[string]string
	received   []byte
	newVersion string        // version to report after a flash; "" = reject image
	offlineFor time.Duration // how long the "reboot" keeps the port closed
	stallRead  bool          // stop reading firmware bytes (simulates a hung printer)
}

func newFakePrinter(t *testing.T, vars map[string]string) *fakePrinter {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	f := &fakePrinter{t: t, addr: l.Addr().String(), l: l, vars: map[string]string{}, offlineFor: 50 * time.Millisecond}
	for k, v := range vars {
		f.vars[k] = v
	}
	t.Cleanup(func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		f.l.Close()
	})
	go f.serve(l)
	return f
}

func (f *fakePrinter) printer() *Printer {
	host, port, _ := net.SplitHostPort(f.addr)
	n, _ := strconv.Atoi(port)
	return &Printer{
		Host:              host,
		Port:              n,
		DialTimeout:       time.Second,
		QueryTimeout:      time.Second,
		WriteStallTimeout: time.Second,
		RebootTimeout:     2 * time.Second,
		PollInterval:      10 * time.Millisecond,
	}
}

func (f *fakePrinter) set(name, value string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.vars[name] = value
}

func (f *fakePrinter) serve(l net.Listener) {
	for {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		go f.handle(conn)
	}
}

func (f *fakePrinter) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	if head, err := r.Peek(len("! PROGRAM")); err == nil && string(head) == "! PROGRAM" {
		f.receiveFirmware(r)
		return
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		name, ok := parseGetVar(line)
		if !ok {
			continue
		}
		f.mu.Lock()
		v, known := f.vars[name]
		f.mu.Unlock()
		if !known {
			v = "?"
		}
		conn.Write([]byte(`"` + v + `"`))
	}
}

func (f *fakePrinter) receiveFirmware(r io.Reader) {
	f.mu.Lock()
	stall := f.stallRead
	f.mu.Unlock()
	if stall {
		time.Sleep(5 * time.Second)
		return
	}
	data, _ := io.ReadAll(r)

	f.mu.Lock()
	f.received = data
	f.l.Close()
	if f.newVersion != "" {
		f.vars["appl.name"] = f.newVersion
	}
	offline := f.offlineFor
	f.mu.Unlock()

	time.Sleep(offline)
	l, err := net.Listen("tcp", f.addr)
	if err != nil {
		f.t.Errorf("fake printer could not come back on %s: %v", f.addr, err)
		return
	}
	f.mu.Lock()
	f.l = l
	f.mu.Unlock()
	go f.serve(l)
}

func parseGetVar(line string) (string, bool) {
	rest, ok := strings.CutPrefix(strings.TrimSpace(line), `! U1 getvar "`)
	if !ok {
		return "", false
	}
	name, _, ok := strings.Cut(rest, `"`)
	return name, ok
}

func (f *fakePrinter) receivedBytes() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	return bytes.Clone(f.received)
}
