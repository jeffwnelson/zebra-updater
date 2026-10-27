package zebra

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"
)

// Default settings used by New and in place of zero-valued Printer fields.
const (
	DefaultPort              = 9100
	DefaultDialTimeout       = 5 * time.Second
	DefaultQueryTimeout      = 10 * time.Second
	DefaultWriteStallTimeout = 2 * time.Minute
	DefaultRebootTimeout     = 10 * time.Minute
	DefaultPollInterval      = 2 * time.Second
)

// Printer is a Link-OS printer reachable over raw TCP. Zero-valued fields
// fall back to the Default* constants.
type Printer struct {
	Host string
	Port int

	// DialTimeout bounds each TCP connect.
	DialTimeout time.Duration
	// QueryTimeout bounds each SGD request/response exchange.
	QueryTimeout time.Duration
	// WriteStallTimeout is how long a firmware write may block before the
	// transfer is abandoned. The printer can stop reading for a while as it
	// processes the image, so this is deliberately much longer than a
	// connect timeout.
	WriteStallTimeout time.Duration
	// RebootTimeout bounds the wait for the printer to flash, reboot, and
	// report the new version.
	RebootTimeout time.Duration
	// PollInterval is the delay between reachability probes while waiting
	// for the reboot.
	PollInterval time.Duration
}

// New returns a Printer for host with default settings.
func New(host string) *Printer {
	return &Printer{Host: host}
}

func (p *Printer) addr() string {
	port := p.Port
	if port == 0 {
		port = DefaultPort
	}
	return net.JoinHostPort(p.Host, strconv.Itoa(port))
}

func orDefault(d, def time.Duration) time.Duration {
	if d <= 0 {
		return def
	}
	return d
}

func (p *Printer) dial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: orDefault(p.DialTimeout, DefaultDialTimeout)}
	conn, err := d.DialContext(ctx, "tcp", p.addr())
	if err != nil {
		return nil, fmt.Errorf("zebra: connect %s: %w", p.addr(), err)
	}
	return conn, nil
}

// Ping reports whether the printer accepts a TCP connection.
func (p *Printer) Ping(ctx context.Context) error {
	conn, err := p.dial(ctx)
	if err != nil {
		return err
	}
	return conn.Close()
}

// session is one connection used for a series of SGD queries.
type session struct {
	p    *Printer
	conn net.Conn
	r    *bufio.Reader
	stop func() bool
}

func (p *Printer) open(ctx context.Context) (*session, error) {
	conn, err := p.dial(ctx)
	if err != nil {
		return nil, err
	}
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	return &session{p: p, conn: conn, r: bufio.NewReader(conn), stop: stop}, nil
}

func (s *session) close() {
	s.stop()
	s.conn.Close()
}

func (s *session) getVar(name string) (string, error) {
	s.conn.SetDeadline(time.Now().Add(orDefault(s.p.QueryTimeout, DefaultQueryTimeout)))
	if err := writeGetVar(s.conn, name); err != nil {
		return "", fmt.Errorf("zebra: getvar %s: %w", name, err)
	}
	v, err := readValue(s.r)
	if err != nil {
		return "", fmt.Errorf("zebra: getvar %s: %w", name, err)
	}
	return v, nil
}

// GetVar queries a single SGD variable. It returns an error wrapping
// ErrUnknownVariable if the printer does not recognize name.
func (p *Printer) GetVar(ctx context.Context, name string) (string, error) {
	s, err := p.open(ctx)
	if err != nil {
		return "", err
	}
	defer s.close()
	return s.getVar(name)
}

// Info is a snapshot of the printer state relevant to firmware upgrades.
type Info struct {
	Model         string // device.product_name, e.g. "ZD621"
	Firmware      string // appl.name, e.g. "V93.21.07Z"
	LinkOSVersion string // appl.link_os_version, e.g. "6.3"
	UniqueID      string // device.unique_id (serial number)
	// Hostname is ip.hostname, which only exists on Link-OS 7.6 and later.
	// It is empty on older firmware.
	Hostname               string
	AllowFirmwareDownloads bool
	HostStatus             HostStatus
}

// Branch is the OS branch of the running firmware, e.g. "V93".
func (i Info) Branch() string { return branch(i.Firmware) }

// Info queries the printer's identity, firmware, and status.
func (p *Printer) Info(ctx context.Context) (Info, error) {
	s, err := p.open(ctx)
	if err != nil {
		return Info{}, err
	}
	defer s.close()

	var info Info
	var allow, status string
	vars := []struct {
		name     string
		dst      *string
		optional bool
	}{
		{"device.product_name", &info.Model, false},
		{"appl.name", &info.Firmware, false},
		{"appl.link_os_version", &info.LinkOSVersion, true},
		{"device.unique_id", &info.UniqueID, true},
		{"ip.hostname", &info.Hostname, true},
		{"device.allow_firmware_downloads", &allow, true},
		{"device.host_status", &status, false},
	}
	for _, v := range vars {
		val, err := s.getVar(v.name)
		if err != nil && !(v.optional && errors.Is(err, ErrUnknownVariable)) {
			return Info{}, err
		}
		*v.dst = val
	}
	info.AllowFirmwareDownloads = allow != "no"
	if info.HostStatus, err = ParseHostStatus(status); err != nil {
		return Info{}, err
	}
	return info, nil
}
