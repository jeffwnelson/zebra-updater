package printer

import (
	"errors"
	"fmt"
	"strings"
)

// BuildGetVar returns the SGD wire-format request for a getvar query.
func BuildGetVar(name string) string {
	return fmt.Sprintf("! U1 getvar %q\r\n", name)
}

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
