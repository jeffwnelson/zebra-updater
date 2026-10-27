package zebra

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// ErrNotFirmware is returned by OpenFirmware when the file lacks the
// "! PROGRAM" header that Link-OS firmware images start with.
var ErrNotFirmware = errors.New("zebra: not a Link-OS firmware image")

var versionRe = regexp.MustCompile(`^V\d+\.\d+\.\d+[A-Z]?$`)

// Firmware is an opened .zpl firmware image. Its header looks like:
//
//	! PROGRAM\n
//	V93.21.53Z\n
//	...signed payload...
type Firmware struct {
	f       *os.File
	size    int64
	version string
}

// OpenFirmware opens a firmware image and reads its declared version.
func OpenFirmware(path string) (*Firmware, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	version, err := readFirmwareHeader(io.NewSectionReader(f, 0, st.Size()))
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &Firmware{f: f, size: st.Size(), version: version}, nil
}

func readFirmwareHeader(r io.Reader) (string, error) {
	br := bufio.NewReaderSize(io.LimitReader(r, 256), 256)
	magic, err := br.ReadString('\n')
	if err != nil || strings.TrimSpace(magic) != "! PROGRAM" {
		return "", ErrNotFirmware
	}
	line, err := br.ReadString('\n')
	if err != nil {
		return "", ErrNotFirmware
	}
	version := strings.TrimSpace(line)
	if !versionRe.MatchString(version) {
		return "", fmt.Errorf("%w: unrecognized version %q", ErrNotFirmware, version)
	}
	return version, nil
}

// Version is the firmware version the image declares, e.g. "V93.21.53Z".
func (fw *Firmware) Version() string { return fw.version }

// Branch is the OS branch of the image, e.g. "V93".
func (fw *Firmware) Branch() string { return branch(fw.version) }

// Size is the image size in bytes.
func (fw *Firmware) Size() int64 { return fw.size }

// Close releases the underlying file.
func (fw *Firmware) Close() error { return fw.f.Close() }

func (fw *Firmware) reader() io.Reader { return io.NewSectionReader(fw.f, 0, fw.size) }

// branch returns the part of a Link-OS version before the first dot.
func branch(version string) string {
	b, _, _ := strings.Cut(version, ".")
	return b
}

// compareVersions orders two versions on the same branch by their numeric
// build components, returning -1, 0, or 1. ok is false if either version
// cannot be parsed.
func compareVersions(a, b string) (cmp int, ok bool) {
	pa, oka := buildNumbers(a)
	pb, okb := buildNumbers(b)
	if !oka || !okb {
		return 0, false
	}
	for i := range pa {
		switch {
		case pa[i] < pb[i]:
			return -1, true
		case pa[i] > pb[i]:
			return 1, true
		}
	}
	return 0, true
}

func buildNumbers(version string) ([2]int, bool) {
	var out [2]int
	parts := strings.Split(strings.TrimRight(version, "ABCDEFGHIJKLMNOPQRSTUVWXYZ"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, s := range parts[1:] {
		n, err := strconv.Atoi(s)
		if err != nil {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
