# Zebra Firmware Updater (`zfw`) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go CLI (`zfw`) that upgrades Zebra ZD621 printer firmware over raw TCP port 9100 from macOS, replacing the Windows-only ZDownloader.exe.

**Architecture:** Single Go module. Two internal packages: `printer` (TCP + SGD wire protocol) and `firmware` (parses `.zpl` firmware files). A thin `cmd` package built on Cobra exposes two verbs: `info` and `upgrade`. All packages developed test-first; protocol code tested against an in-process fake TCP server bound to `127.0.0.1:0`.

**Tech Stack:** Go 1.22+, Cobra v1.8, stdlib `net`, `io`, `bufio`, `testing`. No other runtime dependencies.

---

## File Structure

```
zebra-updater/
├── main.go                          # thin: cmd.Execute()
├── go.mod / go.sum
├── Makefile                         # build, test, e2e targets
├── README.md
├── .gitignore
├── cmd/
│   ├── root.go                      # Cobra root, global flags, exit code helper
│   ├── info.go                      # `zfw info <host>`
│   └── upgrade.go                   # `zfw upgrade <host> <file>`
├── internal/
│   ├── printer/
│   │   ├── sgd.go                   # SGD framing: BuildGetVar, ParseResponse
│   │   ├── sgd_test.go
│   │   ├── printer.go               # Printer type, Connect/GetVar/Send/Ping
│   │   ├── printer_test.go
│   │   └── fake_test.go             # in-process fake SGD server for tests
│   └── firmware/
│       ├── firmware.go              # File type, header parse, Model/Version/Size
│       ├── firmware_test.go
│       └── testdata/
│           └── synthetic-header.bin # crafted header until real sample available
└── .github/workflows/
    └── ci.yml                       # go test, go vet, staticcheck
```

Each file has one responsibility. `cmd/*` files contain only CLI wiring; logic lives in `internal/`.

---

## Task 1: Repository bootstrap

**Files:**
- Create: `go.mod`
- Create: `main.go`
- Create: `.gitignore`
- Create: `Makefile`
- Create: `README.md`
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Initialize Go module**

Run from repo root:

```bash
go mod init github.com/jeffwnelson/zebra-updater
go get github.com/spf13/cobra@v1.8.1
```

- [ ] **Step 2: Create `main.go`**

```go
package main

import "github.com/jeffwnelson/zebra-updater/cmd"

func main() {
	cmd.Execute()
}
```

- [ ] **Step 3: Create `.gitignore`**

```
zfw
/dist/
*.test
*.out
.DS_Store
```

- [ ] **Step 4: Create `Makefile`**

```makefile
.PHONY: build test vet e2e clean

build:
	go build -o zfw .

test:
	go test ./...

vet:
	go vet ./...

e2e:
	@test -n "$$ZFW_E2E_HOST" || (echo "set ZFW_E2E_HOST=hostname"; exit 1)
	go run . info $$ZFW_E2E_HOST

clean:
	rm -f zfw
```

- [ ] **Step 5: Create skeleton `cmd/root.go` so the project compiles**

```go
package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagPort           int
	flagConnectTimeout time.Duration
	flagRebootTimeout  time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "zfw",
	Short: "Upgrade Zebra printer firmware over the network",
}

func init() {
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", 9100, "printer TCP port")
	rootCmd.PersistentFlags().DurationVar(&flagConnectTimeout, "connect-timeout", 5*time.Second, "TCP connect timeout")
	rootCmd.PersistentFlags().DurationVar(&flagRebootTimeout, "reboot-timeout", 5*time.Minute, "max time to wait for printer reboot")
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

- [ ] **Step 6: Create minimal `README.md`**

```markdown
# zebra-updater (`zfw`)

A cross-platform CLI that upgrades Zebra ZD621 printer firmware over raw TCP
port 9100. Replacement for the Windows-only ZDownloader.exe.

See `docs/superpowers/specs/` for the design and `docs/superpowers/plans/` for
the implementation plan.

## Usage (planned)

```
zfw info <hostname>
zfw upgrade <hostname> <firmware.zpl>
```

## Build

```
make build
```
```

- [ ] **Step 7: Create `.github/workflows/ci.yml`**

```yaml
name: ci
on: [push, pull_request]
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
      - run: go vet ./...
      - run: go test ./...
      - run: go build ./...
```

- [ ] **Step 8: Verify it builds**

Run: `go build ./...`
Expected: exits 0, produces no output.

- [ ] **Step 9: Commit**

```bash
git add -A
git commit -m "Bootstrap Go module, Cobra skeleton, CI"
```

---

## Task 2: SGD request framing

SGD (Set/Get/Do) is Zebra's printer config protocol. A `getvar` request looks like:

```
! U1 getvar "device.product.name"\r\n
```

and the response is a single quoted string terminated by `\r\n`, e.g.:

```
"ZD621-203dpi"\r\n
```

We need to build requests and parse responses, including handling embedded escaped quotes (`\"`).

**Files:**
- Create: `internal/printer/sgd.go`
- Create: `internal/printer/sgd_test.go`

- [ ] **Step 1: Write failing test for `BuildGetVar`**

`internal/printer/sgd_test.go`:

```go
package printer

import "testing"

func TestBuildGetVar(t *testing.T) {
	got := BuildGetVar("device.product.name")
	want := "! U1 getvar \"device.product.name\"\r\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
```

- [ ] **Step 2: Run test — should fail to compile**

Run: `go test ./internal/printer/...`
Expected: build error, `undefined: BuildGetVar`.

- [ ] **Step 3: Implement `BuildGetVar`**

`internal/printer/sgd.go`:

```go
package printer

import "fmt"

// BuildGetVar returns the SGD wire-format request for a getvar query.
func BuildGetVar(name string) string {
	return fmt.Sprintf("! U1 getvar %q\r\n", name)
}
```

- [ ] **Step 4: Run test — should pass**

Run: `go test ./internal/printer/...`
Expected: `PASS`.

- [ ] **Step 5: Write failing tests for `ParseResponse`**

Append to `sgd_test.go`:

```go
func TestParseResponse(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        bool
	}{
		{"simple", "\"ZD621-203dpi\"\r\n", "ZD621-203dpi", false},
		{"empty", "\"\"\r\n", "", false},
		{"escaped quote", "\"a\\\"b\"\r\n", "a\"b", false},
		{"no quotes", "ZD621\r\n", "", true},
		{"unterminated", "\"ZD621", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseResponse(c.in)
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 6: Run tests — should fail**

Run: `go test ./internal/printer/...`
Expected: build error, `undefined: ParseResponse`.

- [ ] **Step 7: Implement `ParseResponse`**

Append to `sgd.go`:

```go
import (
	"errors"
	"strings"
)

// ParseResponse extracts the value from a quoted SGD response line.
// Input is the full line including trailing CRLF; quotes inside the value
// may be escaped with backslash.
func ParseResponse(line string) (string, error) {
	line = strings.TrimRight(line, "\r\n")
	if len(line) < 2 || line[0] != '"' || line[len(line)-1] != '"' {
		return "", errors.New("malformed SGD response: missing surrounding quotes")
	}
	body := line[1 : len(line)-1]
	return strings.ReplaceAll(body, `\"`, `"`), nil
}
```

Then fix the imports block at the top of `sgd.go` to read:

```go
import (
	"errors"
	"fmt"
	"strings"
)
```

- [ ] **Step 8: Run tests — should pass**

Run: `go test ./internal/printer/...`
Expected: all cases `PASS`.

- [ ] **Step 9: Commit**

```bash
git add internal/printer/sgd.go internal/printer/sgd_test.go
git commit -m "Add SGD request builder and response parser"
```

---

## Task 3: Fake SGD server for tests

We need an in-process TCP server that speaks SGD so we can test `Printer.Connect`, `GetVar`, and `Ping` without real hardware.

**Files:**
- Create: `internal/printer/fake_test.go`

- [ ] **Step 1: Create the fake server**

`internal/printer/fake_test.go`:

```go
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
```

- [ ] **Step 2: Verify the file builds (no test cases yet)**

Run: `go test ./internal/printer/...`
Expected: existing SGD tests still PASS, no new failures.

- [ ] **Step 3: Commit**

```bash
git add internal/printer/fake_test.go
git commit -m "Add in-process fake SGD server for tests"
```

---

## Task 4: Printer.Connect and Printer.GetVar

**Files:**
- Create: `internal/printer/printer.go`
- Modify: `internal/printer/printer_test.go` (new file)

- [ ] **Step 1: Write failing test for `GetVar`**

`internal/printer/printer_test.go`:

```go
package printer

import (
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
```

- [ ] **Step 2: Run tests — should fail to compile**

Run: `go test ./internal/printer/...`
Expected: build error, `undefined: Printer`.

- [ ] **Step 3: Implement `Printer.GetVar`**

`internal/printer/printer.go`:

```go
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
```

- [ ] **Step 4: Run tests — should pass**

Run: `go test ./internal/printer/...`
Expected: all 3 cases `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/printer/printer.go internal/printer/printer_test.go
git commit -m "Add Printer.GetVar over single TCP connection"
```

---

## Task 5: Printer.Ping

A non-destructive reachability check for the reboot detection loop. Just attempt a TCP connect; close immediately.

**Files:**
- Modify: `internal/printer/printer.go`
- Modify: `internal/printer/printer_test.go`

- [ ] **Step 1: Write failing test**

Append to `printer_test.go`:

```go
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
```

- [ ] **Step 2: Run tests — should fail to compile**

Run: `go test ./internal/printer/...`
Expected: build error, `p.Ping undefined`.

- [ ] **Step 3: Implement `Ping`**

Append to `printer.go`:

```go
// Ping verifies the printer accepts a TCP connection on the configured port.
func (p *Printer) Ping() error {
	conn, err := p.dial()
	if err != nil {
		return err
	}
	return conn.Close()
}
```

- [ ] **Step 4: Run tests — should pass**

Run: `go test ./internal/printer/...`
Expected: all cases `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/printer/printer.go internal/printer/printer_test.go
git commit -m "Add Printer.Ping for reachability checks"
```

---

## Task 6: Printer.Send (firmware streaming)

Streams an arbitrary `io.Reader` to the printer over port 9100. Reports bytes written via a callback so the CLI can render progress.

**Files:**
- Modify: `internal/printer/printer.go`
- Modify: `internal/printer/printer_test.go`

- [ ] **Step 1: Write failing tests**

Append to `printer_test.go`:

```go
import (
	"bytes"
	"net"
)

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
```

- [ ] **Step 2: Run tests — should fail to compile**

Run: `go test ./internal/printer/...`
Expected: build error, `p.Send undefined`.

- [ ] **Step 3: Implement `Send`**

Append to `printer.go`:

```go
import "io"

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
```

Add `"io"` to the import block at the top of `printer.go`.

- [ ] **Step 4: Run tests — should pass**

Run: `go test ./internal/printer/...`
Expected: all cases `PASS`.

- [ ] **Step 5: Commit**

```bash
git add internal/printer/printer.go internal/printer/printer_test.go
git commit -m "Add Printer.Send to stream firmware bytes with progress"
```

---

## Task 7: Firmware file inspection

The `.zpl` firmware file is a binary blob. We need its size (always) and ideally the model + version it targets (for pre-flight matching). The exact header format is documented sparsely; v1 parser:

- Always returns `Size()` from `os.Stat`.
- Tries to extract `Model()` and `Version()` from the first 4 KiB by scanning for ASCII patterns matching `ZD\d{3}` and `V\d{2}\.\d{2}\.\d{2}[A-Z]?`.
- Returns empty string + nil error if not found (caller must accept user-supplied `--target-model` / `--target-version` flags as fallback).

**Files:**
- Create: `internal/firmware/firmware.go`
- Create: `internal/firmware/firmware_test.go`
- Create: `internal/firmware/testdata/synthetic-header.bin`

- [ ] **Step 1: Create synthetic test fixture**

Use a short Go program or shell one-liner. From repo root:

```bash
mkdir -p internal/firmware/testdata
printf 'BLOBHEADER\x00\x01ZD621-203dpi firmware V99.01.02Z trailing-junk\x00\x00' \
  > internal/firmware/testdata/synthetic-header.bin
```

Verify size > 0:

```bash
ls -l internal/firmware/testdata/synthetic-header.bin
```

- [ ] **Step 2: Write failing tests**

`internal/firmware/firmware_test.go`:

```go
package firmware

import "testing"

func TestOpenAndInspectSynthetic(t *testing.T) {
	f, err := Open("testdata/synthetic-header.bin")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	if f.Size() == 0 {
		t.Error("Size() returned 0")
	}
	if got := f.Model(); got != "ZD621" {
		t.Errorf("Model() = %q, want ZD621", got)
	}
	if got := f.Version(); got != "V99.01.02Z" {
		t.Errorf("Version() = %q, want V99.01.02Z", got)
	}
}

func TestOpenNonexistent(t *testing.T) {
	if _, err := Open("testdata/does-not-exist.bin"); err == nil {
		t.Error("expected error opening missing file")
	}
}
```

- [ ] **Step 3: Run tests — should fail to compile**

Run: `go test ./internal/firmware/...`
Expected: build error, `undefined: Open`.

- [ ] **Step 4: Implement firmware parser**

`internal/firmware/firmware.go`:

```go
package firmware

import (
	"io"
	"os"
	"regexp"
)

var (
	modelRe   = regexp.MustCompile(`ZD\d{3}`)
	versionRe = regexp.MustCompile(`V\d{2}\.\d{2}\.\d{2}[A-Z]?`)
)

type File struct {
	path    string
	size    int64
	model   string
	version string
	f       *os.File
}

// Open reads the file's header (up to 4 KiB) to extract model and version,
// then leaves the file open for streaming via Reader().
func Open(path string) (*File, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	header := make([]byte, 4096)
	n, err := io.ReadFull(f, header)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		f.Close()
		return nil, err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	out := &File{path: path, size: st.Size(), f: f}
	if m := modelRe.Find(header[:n]); m != nil {
		out.model = string(m)
	}
	if v := versionRe.Find(header[:n]); v != nil {
		out.version = string(v)
	}
	return out, nil
}

func (f *File) Size() int64     { return f.size }
func (f *File) Model() string   { return f.model }
func (f *File) Version() string { return f.version }
func (f *File) Reader() io.Reader { return f.f }
func (f *File) Close() error    { return f.f.Close() }
```

- [ ] **Step 5: Run tests — should pass**

Run: `go test ./internal/firmware/...`
Expected: both cases `PASS`.

- [ ] **Step 6: Commit**

```bash
git add internal/firmware/ internal/firmware/testdata/
git commit -m "Add firmware file parser with header pattern extraction"
```

---

## Task 8: `zfw info` command

**Files:**
- Create: `cmd/info.go`
- Modify: `cmd/root.go` (register info command)

- [ ] **Step 1: Implement `cmd/info.go`**

```go
package cmd

import (
	"fmt"
	"os"

	"github.com/jeffwnelson/zebra-updater/internal/printer"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <host>",
	Short: "Show printer model, firmware, free flash, and host status",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		host := args[0]
		p := &printer.Printer{Host: host, Port: flagPort, Timeout: flagConnectTimeout}

		fields := []struct{ label, varName string }{
			{"Model         ", "device.product.name"},
			{"Firmware      ", "appl.name"},
			{"Free flash    ", "memory.flash_free"},
			{"Host status   ", "device.host_status"},
			{"Link status   ", "ip.addr"},
		}
		for _, fld := range fields {
			val, err := p.GetVar(fld.varName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error querying %s: %v\n", fld.varName, err)
				os.Exit(1)
			}
			fmt.Printf("%s %s\n", fld.label, val)
		}
	},
}
```

- [ ] **Step 2: Register `infoCmd` in `cmd/root.go`**

Inside the existing `init()` function in `cmd/root.go`, append:

```go
	rootCmd.AddCommand(infoCmd)
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 4: Manual smoke test against a real printer**

Run: `go run . info printer.example.com`
Expected: prints five labeled fields. Note any that come back empty for the spec's open-questions section.

- [ ] **Step 5: Commit**

```bash
git add cmd/info.go cmd/root.go
git commit -m "Add zfw info command"
```

---

## Task 9: `zfw upgrade` — pre-flight checks

This task wires the upgrade subcommand with all five pre-flight checks but stops before sending firmware. The actual push and reboot detection come in Task 10.

**Files:**
- Create: `cmd/upgrade.go`
- Modify: `cmd/root.go` (register upgrade command)

- [ ] **Step 1: Implement `cmd/upgrade.go` (pre-flight only)**

```go
package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jeffwnelson/zebra-updater/internal/firmware"
	"github.com/jeffwnelson/zebra-updater/internal/printer"
	"github.com/spf13/cobra"
)

const (
	exitPreflight = 2
	exitTransfer  = 3
	exitReboot    = 4
)

var (
	flagDryRun        bool
	flagYes           bool
	flagForce         bool
	flagTargetModel   string
	flagTargetVersion string
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade <host> <firmware.zpl>",
	Short: "Upgrade firmware on a Zebra printer",
	Args:  cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		host, fwPath := args[0], args[1]
		runUpgrade(host, fwPath)
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "run pre-flight only, do not send firmware")
	upgradeCmd.Flags().BoolVar(&flagYes, "yes", false, "skip confirmation prompt")
	upgradeCmd.Flags().BoolVar(&flagForce, "force", false, "upgrade even if firmware version matches")
	upgradeCmd.Flags().StringVar(&flagTargetModel, "target-model", "", "override model declared by firmware header (fallback if parser cannot extract)")
	upgradeCmd.Flags().StringVar(&flagTargetVersion, "target-version", "", "override version declared by firmware header")
}

func runUpgrade(host, fwPath string) {
	fw, err := firmware.Open(fwPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open firmware: %v\n", err)
		os.Exit(exitPreflight)
	}
	defer fw.Close()

	targetModel := fw.Model()
	if flagTargetModel != "" {
		targetModel = flagTargetModel
	}
	targetVersion := fw.Version()
	if flagTargetVersion != "" {
		targetVersion = flagTargetVersion
	}
	if targetModel == "" || targetVersion == "" {
		fmt.Fprintln(os.Stderr, "pre-flight: firmware header could not be parsed; provide --target-model and --target-version")
		os.Exit(exitPreflight)
	}

	p := &printer.Printer{Host: host, Port: flagPort, Timeout: flagConnectTimeout}

	if err := p.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: cannot reach %s:%d: %v\n", host, flagPort, err)
		os.Exit(exitPreflight)
	}

	model, err := p.GetVar("device.product.name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query model: %v\n", err)
		os.Exit(exitPreflight)
	}
	if !strings.Contains(model, targetModel) {
		fmt.Fprintf(os.Stderr, "pre-flight: model mismatch — file targets %s, printer reports %s\n", targetModel, model)
		os.Exit(exitPreflight)
	}

	current, err := p.GetVar("appl.name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query version: %v\n", err)
		os.Exit(exitPreflight)
	}
	if current == targetVersion && !flagForce {
		fmt.Printf("already on %s; nothing to do (use --force to reflash)\n", targetVersion)
		os.Exit(0)
	}

	free, err := p.GetVar("memory.flash_free")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query free flash: %v\n", err)
		os.Exit(exitPreflight)
	}
	freeBytes, err := strconv.ParseInt(strings.TrimSpace(free), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: unparseable memory.flash_free=%q: %v\n", free, err)
		os.Exit(exitPreflight)
	}
	need := int64(float64(fw.Size()) * 1.2)
	if freeBytes < need {
		fmt.Fprintf(os.Stderr, "pre-flight: low flash — have %d bytes, need ≥ %d\n", freeBytes, need)
		os.Exit(exitPreflight)
	}

	status, err := p.GetVar("device.host_status")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query host status: %v\n", err)
		os.Exit(exitPreflight)
	}
	// The first whitespace-separated field of host_status is the number of formats
	// in receive buffer; a nonzero value means the printer is busy. We treat any
	// non-"0" first token as busy.
	if first := strings.Fields(status); len(first) > 0 && first[0] != "0" {
		fmt.Fprintf(os.Stderr, "pre-flight: printer busy (host_status=%q)\n", status)
		os.Exit(exitPreflight)
	}

	fmt.Printf("Pre-flight OK: %s — %s → %s, %d bytes\n", model, current, targetVersion, fw.Size())

	if flagDryRun {
		return
	}

	if !flagYes {
		fmt.Print("Proceed with upgrade? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("aborted")
			return
		}
	}

	// Task 10 will fill in the actual upgrade flow here.
	fmt.Println("(upgrade flow not yet implemented — see Task 10)")
}
```

- [ ] **Step 2: Register `upgradeCmd` in `cmd/root.go`**

Inside `init()` in `cmd/root.go`, append:

```go
	rootCmd.AddCommand(upgradeCmd)
```

- [ ] **Step 3: Verify build**

Run: `go build ./...`
Expected: exits 0.

- [ ] **Step 4: Manual smoke test (dry run)**

Get a real firmware file (or copy any binary file as a stand-in to exercise the failure path). Then:

```bash
go run . upgrade printer.example.com ./some-firmware.zpl --dry-run --target-model ZD621 --target-version V99.99.99Z
```

Expected: prints a pre-flight failure for version mismatch (or success line if pre-flight passes), no firmware sent.

- [ ] **Step 5: Commit**

```bash
git add cmd/upgrade.go cmd/root.go
git commit -m "Add zfw upgrade with pre-flight checks (no transfer yet)"
```

---

## Task 10: `zfw upgrade` — push and reboot detection

Replaces the placeholder at the end of `runUpgrade` with the actual transfer, reboot polling, and post-reboot version check.

**Files:**
- Modify: `cmd/upgrade.go`

- [ ] **Step 1: Replace the placeholder with full transfer + reboot logic**

In `cmd/upgrade.go`, replace the two lines:

```go
	// Task 10 will fill in the actual upgrade flow here.
	fmt.Println("(upgrade flow not yet implemented — see Task 10)")
```

with:

```go
	if err := pushFirmware(p, fw); err != nil {
		fmt.Fprintf(os.Stderr, "transfer failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "WARNING: printer may be in a partially flashed state — run `zfw info` to inspect before retrying")
		os.Exit(exitTransfer)
	}

	if err := waitForReboot(p, flagRebootTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "reboot wait: %v\n", err)
		fmt.Fprintln(os.Stderr, "Firmware was sent. Power-cycle the printer and run `zfw info` to confirm state.")
		os.Exit(exitReboot)
	}

	after, err := p.GetVar("appl.name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "post-reboot version query failed: %v\n", err)
		os.Exit(exitReboot)
	}
	fmt.Printf("Upgrade complete: %s → %s\n", current, after)
	if after != targetVersion {
		fmt.Fprintf(os.Stderr, "WARNING: post-reboot version %q does not match target %q\n", after, targetVersion)
		os.Exit(exitReboot)
	}
```

- [ ] **Step 2: Add `pushFirmware` and `waitForReboot` helpers**

Append to `cmd/upgrade.go`:

```go
import (
	"time"
)

func pushFirmware(p *printer.Printer, fw *firmware.File) error {
	total := fw.Size()
	last := time.Now()
	return p.Send(fw.Reader(), func(written int64) {
		if time.Since(last) < 500*time.Millisecond && written < total {
			return
		}
		last = time.Now()
		fmt.Printf("\r  %.1f / %.1f MiB", float64(written)/(1<<20), float64(total)/(1<<20))
		if written >= total {
			fmt.Println()
		}
	})
}

// waitForReboot polls the printer's TCP port until we observe a "down" sample
// followed by an "up" sample, indicating the flash cycle completed.
func waitForReboot(p *printer.Printer, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	sawDown := false
	for time.Now().Before(deadline) {
		err := p.Ping()
		if err != nil && !sawDown {
			fmt.Println("Printer is flashing...")
			sawDown = true
		}
		if err == nil && sawDown {
			fmt.Println("Printer is back online.")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	if !sawDown {
		return fmt.Errorf("printer never went offline within %s; flash may not have started", timeout)
	}
	return fmt.Errorf("printer did not come back online within %s", timeout)
}
```

Add `"time"` to the existing import block at the top of `cmd/upgrade.go` if not already present (it isn't from Task 9, so add it).

- [ ] **Step 3: Verify build and existing tests**

Run: `go build ./... && go test ./...`
Expected: both exit 0.

- [ ] **Step 4: Manual end-to-end test**

ONLY run against a printer you are genuinely upgrading. Use `--dry-run` first to confirm pre-flight passes, then:

```bash
go run . upgrade printer.example.com ./firmware.zpl
```

Expected: pre-flight prints OK, prompts for confirmation, streams firmware with progress, prints "flashing...", prints "back online", prints final `before → after` version.

- [ ] **Step 5: Commit**

```bash
git add cmd/upgrade.go
git commit -m "Implement firmware push, reboot detection, and post-flash verification"
```

---

## Task 11: README polish

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Replace `README.md` with a usable version**

```markdown
# zebra-updater (`zfw`)

A small Go CLI that upgrades Zebra ZD621 printer firmware over raw TCP port
9100. A cross-platform replacement for the Windows-only ZDownloader.exe.

## Why this exists

Zebra ships firmware as `.zpl` files but only distributes a Windows GUI tool
(ZDownloader.exe) to push them. Under the hood, ZDownloader is just streaming
the firmware file at the printer over a TCP socket. This tool does the same
thing from any platform with a Go toolchain — primary target macOS.

## Install

```bash
go install github.com/jeffwnelson/zebra-updater@latest
# binary lands in $(go env GOPATH)/bin as `zebra-updater`
# rename or symlink to `zfw` if you prefer
```

Or from source:

```bash
git clone https://github.com/jeffwnelson/zebra-updater
cd zebra-updater
make build
./zfw --help
```

## Usage

Inspect a printer:

```bash
zfw info printer.example.com
```

Upgrade firmware (with confirmation prompt):

```bash
zfw upgrade printer.example.com ./ZD621_V99.21.04Z.zpl
```

Non-interactive:

```bash
zfw upgrade printer.example.com ./firmware.zpl --yes
```

Pre-flight only (no firmware sent):

```bash
zfw upgrade printer.example.com ./firmware.zpl --dry-run
```

If the firmware header can't be auto-parsed, supply the targets explicitly:

```bash
zfw upgrade printer.example.com ./firmware.zpl \
  --target-model ZD621 --target-version V99.21.04Z
```

## Exit codes

| Code | Meaning                                                     |
|------|-------------------------------------------------------------|
| 0    | Success                                                     |
| 2    | Pre-flight failure (no bytes sent)                          |
| 3    | Transfer failure (firmware partially or fully sent, unsafe) |
| 4    | Reboot timeout (firmware sent, printer not back in time)    |

## Safety

- Pre-flight refuses model mismatch, version match (without `--force`), low
  flash, and busy printers.
- On transfer failure, the tool does **not** retry. Run `zfw info` to inspect
  the printer's state before attempting another upgrade.
- Only tested against ZD621. Other Link-OS printers may work but are not
  validated.

## Design and plan

See `docs/superpowers/specs/` and `docs/superpowers/plans/`.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "Flesh out README with usage, exit codes, safety notes"
```

---

## Self-review notes

- **Spec coverage:** Architecture (Task 1), SGD protocol (Tasks 2–3), Printer methods (Tasks 4–6), firmware parsing (Task 7), CLI surface — info (Task 8) and upgrade (Tasks 9–10), exit codes (defined in Task 9, used in Tasks 9–10), README (Task 11). All spec sections accounted for.
- **Open question deferred to user verification:** The spec calls out that the firmware header format isn't fully known. Task 7 uses regex-based extraction with explicit `--target-model` / `--target-version` flags as fallback (added in Task 9), so the tool remains usable even if the parser misses.
- **Test coverage:** SGD framing (Task 2), Printer.GetVar including connect failure (Task 4), Printer.Ping (Task 5), Printer.Send (Task 6), firmware parser (Task 7). End-to-end manual test against a real printer (Tasks 8, 9, 10 step 4). No e2e for the upgrade flow itself per spec — would wear flash memory.
- **Type consistency:** `Printer` struct fields (`Host`, `Port`, `Timeout`) are consistent across tasks. `File` methods (`Size`, `Model`, `Version`, `Reader`, `Close`) referenced in Tasks 9–10 are all defined in Task 7. Exit code constants defined once in Task 9.
