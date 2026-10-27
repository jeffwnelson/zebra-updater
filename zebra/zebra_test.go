package zebra

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

// writeImage writes a synthetic firmware image in the real Link-OS layout.
func writeImage(t *testing.T, version string, payload int) string {
	t.Helper()
	var b bytes.Buffer
	b.WriteString("! PROGRAM\n" + version + "\n~DC1234567;\n1234567\n")
	b.Write(bytes.Repeat([]byte{0xA5}, payload))
	path := filepath.Join(t.TempDir(), version+".zpl")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func openImage(t *testing.T, version string, payload int) *Firmware {
	t.Helper()
	fw, err := OpenFirmware(writeImage(t, version, payload))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { fw.Close() })
	return fw
}

func TestReadValue(t *testing.T) {
	cases := []struct {
		name, in, want string
		wantErr        error
	}{
		{"no trailing newline", `"ZD621"`, "ZD621", nil},
		{"multi-line", "\"030,0\r\n000,0\r\n0000,0\"", "030,0\r\n000,0\r\n0000,0", nil},
		{"trailing spaces", `"yes "`, "yes", nil},
		{"unknown", `"?"`, "", ErrUnknownVariable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := readValue(bufio.NewReader(strings.NewReader(c.in)))
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	if _, err := readValue(bufio.NewReader(strings.NewReader(`"unterminated`))); err == nil {
		t.Error("unterminated value: expected error")
	}
}

func TestParseHostStatus(t *testing.T) {
	st, err := ParseHostStatus(zd621Vars["device.host_status"])
	if err != nil {
		t.Fatal(err)
	}
	if st.Busy() || st.Paused || st.PaperOut || st.HeadOpen {
		t.Errorf("idle ZD621 decoded as %+v", st)
	}

	busy, err := ParseHostStatus("\x02030,0,1,1809,002,0,0,0,000,0,0,0\x03\r\n\x02000,0,1,0,0,2,6,0,00000012,1,000\x03\r\n\x020000,0\x03")
	if err != nil {
		t.Fatal(err)
	}
	if !busy.Busy() || busy.FormatsInBuffer != 2 || busy.LabelsRemaining != 12 || !busy.Paused || !busy.HeadOpen {
		t.Errorf("busy status decoded as %+v", busy)
	}

	if _, err := ParseHostStatus("030,0,0"); err == nil {
		t.Error("truncated status: expected error")
	}
}

func TestOpenFirmware(t *testing.T) {
	fw := openImage(t, "V93.21.53Z", 1024)
	if fw.Version() != "V93.21.53Z" || fw.Branch() != "V93" {
		t.Errorf("got version %q branch %q", fw.Version(), fw.Branch())
	}
	if fw.Size() < 1024 {
		t.Errorf("Size() = %d", fw.Size())
	}

	if fw := openImage(t, "V100.21.53Z", 0); fw.Branch() != "V100" {
		t.Errorf("three-digit branch: got %q", fw.Branch())
	}

	junk := filepath.Join(t.TempDir(), "label.zpl")
	os.WriteFile(junk, []byte("^XA^FDhello^FS^XZ"), 0o644)
	if _, err := OpenFirmware(junk); !errors.Is(err, ErrNotFirmware) {
		t.Errorf("label file: err = %v, want ErrNotFirmware", err)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"V93.21.53Z", "V93.21.07Z", 1},
		{"V93.21.07Z", "V93.21.53Z", -1},
		{"V93.21.53Z", "V93.21.53Z", 0},
		{"V93.22.01Z", "V93.21.53Z", 1},
	}
	for _, c := range cases {
		if got, ok := compareVersions(c.a, c.b); !ok || got != c.want {
			t.Errorf("compareVersions(%s, %s) = %d, %v; want %d", c.a, c.b, got, ok, c.want)
		}
	}
}

func TestInfo(t *testing.T) {
	f := newFakePrinter(t, zd621Vars)
	info, err := f.printer().Info(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if info.Model != "ZD621" || info.Firmware != "V93.21.07Z" || info.Branch() != "V93" {
		t.Errorf("got %+v", info)
	}
	if info.Hostname != "" {
		t.Errorf("ip.hostname is unknown on Link-OS 6.3; got %q", info.Hostname)
	}
	if !info.AllowFirmwareDownloads {
		t.Error("AllowFirmwareDownloads = false")
	}
}

func TestGetVarUnknown(t *testing.T) {
	f := newFakePrinter(t, zd621Vars)
	_, err := f.printer().GetVar(context.Background(), "device.product.name")
	if !errors.Is(err, ErrUnknownVariable) {
		t.Errorf("err = %v, want ErrUnknownVariable", err)
	}
}

func TestCheck(t *testing.T) {
	ctx := context.Background()

	t.Run("ok", func(t *testing.T) {
		r, err := newFakePrinter(t, zd621Vars).printer().Check(ctx, openImage(t, "V93.21.53Z", 0))
		if err != nil {
			t.Fatal(err)
		}
		if !r.OK() || r.UpToDate {
			t.Errorf("report = %+v", r)
		}
	})

	t.Run("branch mismatch", func(t *testing.T) {
		r, _ := newFakePrinter(t, zd621Vars).printer().Check(ctx, openImage(t, "V92.21.53Z", 0))
		if r.OK() || !strings.Contains(r.Problems[0], "branch V92") {
			t.Errorf("problems = %q", r.Problems)
		}
	})

	t.Run("special build", func(t *testing.T) {
		f := newFakePrinter(t, zd621Vars)
		f.set("appl.name", "SP93-006444A")
		r, _ := f.printer().Check(ctx, openImage(t, "V93.21.53Z", 0))
		if r.OK() || !strings.Contains(r.Problems[0], "non-standard firmware SP93-006444A") {
			t.Errorf("problems = %q", r.Problems)
		}
	})

	t.Run("downloads disabled", func(t *testing.T) {
		f := newFakePrinter(t, zd621Vars)
		f.set("device.allow_firmware_downloads", "no")
		r, _ := f.printer().Check(ctx, openImage(t, "V93.21.53Z", 0))
		if r.OK() {
			t.Error("expected a problem")
		}
	})

	t.Run("busy", func(t *testing.T) {
		f := newFakePrinter(t, zd621Vars)
		f.set("device.host_status", "030,0,0,1809,001,0,0,0,000,0,0,0\r\n000,0,0,0,0,2,6,0,00000000,1,000\r\n0000,0")
		r, _ := f.printer().Check(ctx, openImage(t, "V93.21.53Z", 0))
		if r.OK() {
			t.Error("expected a problem")
		}
	})

	t.Run("up to date", func(t *testing.T) {
		r, _ := newFakePrinter(t, zd621Vars).printer().Check(ctx, openImage(t, "V93.21.07Z", 0))
		if !r.OK() || !r.UpToDate {
			t.Errorf("report = %+v", r)
		}
	})

	t.Run("downgrade warns", func(t *testing.T) {
		r, _ := newFakePrinter(t, zd621Vars).printer().Check(ctx, openImage(t, "V93.21.01Z", 0))
		if !r.OK() || len(r.Warnings) == 0 {
			t.Errorf("report = %+v", r)
		}
	})

	t.Run("hostname warning only for names", func(t *testing.T) {
		f := newFakePrinter(t, zd621Vars)
		p := f.printer()
		r, _ := p.Check(ctx, openImage(t, "V93.21.53Z", 0))
		if len(r.Warnings) != 0 {
			t.Errorf("IP address host: unexpected warnings %q", r.Warnings)
		}
		p.Host = "localhost"
		r, err := p.Check(ctx, openImage(t, "V93.21.53Z", 0))
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "hostname") {
			t.Errorf("warnings = %q", r.Warnings)
		}
	})
}

func TestUpgrade(t *testing.T) {
	f := newFakePrinter(t, zd621Vars)
	f.newVersion = "V93.21.53Z"
	fw := openImage(t, "V93.21.53Z", 200*1024)

	var phases []Phase
	var sawOffline bool
	err := f.printer().Upgrade(context.Background(), fw, UpgradeOptions{Progress: func(pr Progress) {
		if len(phases) == 0 || phases[len(phases)-1] != pr.Phase {
			phases = append(phases, pr.Phase)
		}
		sawOffline = sawOffline || pr.Offline
	}})
	if err != nil {
		t.Fatalf("Upgrade: %v", err)
	}

	want, _ := os.ReadFile(fw.f.Name())
	if !bytes.Equal(f.receivedBytes(), want) {
		t.Errorf("printer received %d bytes, want %d", len(f.receivedBytes()), len(want))
	}
	if got := []Phase{PhaseSending, PhaseRebooting, PhaseDone}; !slices.Equal(phases, got) {
		t.Errorf("phases = %v, want %v", phases, got)
	}
	if !sawOffline {
		t.Error("never reported the printer offline")
	}
}

func TestUpgradeRefusals(t *testing.T) {
	ctx := context.Background()

	var ce *CheckError
	err := newFakePrinter(t, zd621Vars).printer().Upgrade(ctx, openImage(t, "V92.21.53Z", 0), UpgradeOptions{})
	if !errors.As(err, &ce) {
		t.Errorf("branch mismatch: err = %v, want *CheckError", err)
	}

	f := newFakePrinter(t, zd621Vars)
	err = f.printer().Upgrade(ctx, openImage(t, "V93.21.07Z", 0), UpgradeOptions{})
	if !errors.Is(err, ErrUpToDate) {
		t.Errorf("same version: err = %v, want ErrUpToDate", err)
	}
	if f.receivedBytes() != nil {
		t.Error("firmware was sent despite refusal")
	}
}

func TestUpgradeRejectedImage(t *testing.T) {
	f := newFakePrinter(t, zd621Vars) // newVersion "" → comes back on old firmware
	p := f.printer()
	p.RebootTimeout = 500 * time.Millisecond
	err := p.Upgrade(context.Background(), openImage(t, "V93.21.53Z", 1024), UpgradeOptions{})
	var vm *VersionMismatchError
	if !errors.As(err, &vm) || vm.Got != "V93.21.07Z" {
		t.Errorf("err = %v, want *VersionMismatchError from V93.21.07Z", err)
	}
}

func TestUpgradeWriteStall(t *testing.T) {
	f := newFakePrinter(t, zd621Vars)
	f.stallRead = true
	p := f.printer()
	p.WriteStallTimeout = 200 * time.Millisecond
	// Big enough to overrun the loopback socket buffers.
	err := p.Upgrade(context.Background(), openImage(t, "V93.21.53Z", 64<<20), UpgradeOptions{})
	var te *TransferError
	if !errors.As(err, &te) {
		t.Fatalf("err = %v, want *TransferError", err)
	}
	if te.Sent == 0 || te.Sent >= te.Total {
		t.Errorf("Sent = %d of %d", te.Sent, te.Total)
	}
}

func TestUpgradeCancel(t *testing.T) {
	f := newFakePrinter(t, zd621Vars)
	f.stallRead = true
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)
	err := f.printer().Upgrade(ctx, openImage(t, "V93.21.53Z", 64<<20), UpgradeOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}
