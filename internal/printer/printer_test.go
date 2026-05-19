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
