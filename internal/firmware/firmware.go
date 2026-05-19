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

func (f *File) Size() int64       { return f.size }
func (f *File) Model() string     { return f.model }
func (f *File) Version() string   { return f.version }
func (f *File) Reader() io.Reader { return f.f }
func (f *File) Close() error      { return f.f.Close() }
