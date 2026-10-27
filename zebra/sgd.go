package zebra

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ErrUnknownVariable is returned when the printer answers a getvar with "?",
// which is how Link-OS reports a variable it does not recognize.
var ErrUnknownVariable = errors.New("zebra: unknown SGD variable")

func writeGetVar(w io.Writer, name string) error {
	_, err := fmt.Fprintf(w, "! U1 getvar \"%s\"\r\n", name)
	return err
}

// readValue reads one quoted SGD response. Link-OS sends the value wrapped in
// double quotes with no trailing newline, and the value itself may span
// several CRLF-separated lines (device.host_status does).
func readValue(r *bufio.Reader) (string, error) {
	if _, err := r.ReadString('"'); err != nil {
		return "", err
	}
	s, err := r.ReadString('"')
	if err != nil {
		return "", err
	}
	v := strings.TrimSpace(s[:len(s)-1])
	if v == "?" {
		return "", ErrUnknownVariable
	}
	return v, nil
}
