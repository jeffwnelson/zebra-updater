package zebra

import (
	"fmt"
	"strconv"
	"strings"
)

// HostStatus is the decoded ~HS host status, as returned by the
// device.host_status SGD variable.
type HostStatus struct {
	PaperOut        bool
	Paused          bool
	FormatsInBuffer int
	BufferFull      bool
	PartialFormat   bool
	HeadOpen        bool
	RibbonOut       bool
	LabelsRemaining int
	Raw             string
}

// Busy reports whether the printer has work queued or in progress.
func (s HostStatus) Busy() bool {
	return s.FormatsInBuffer > 0 || s.LabelsRemaining > 0 || s.PartialFormat
}

// ParseHostStatus decodes the three-line ~HS response:
//
//	aaa,b,c,dddd,eee,f,g,h,iii,j,k,l
//	mmm,n,o,p,q,r,s,t,uuuuuuuu,v,www
//	xxxx,y
//
// Only the first two lines carry fields used here.
func ParseHostStatus(raw string) (HostStatus, error) {
	st := HostStatus{Raw: raw}
	lines := strings.FieldsFunc(raw, func(r rune) bool { return r == '\r' || r == '\n' })
	if len(lines) < 2 {
		return st, fmt.Errorf("zebra: host status has %d lines, want 3", len(lines))
	}
	l1, l2 := splitStatusLine(lines[0]), splitStatusLine(lines[1])
	if len(l1) < 8 || len(l2) < 9 {
		return st, fmt.Errorf("zebra: malformed host status %q", raw)
	}
	var err error
	if st.FormatsInBuffer, err = strconv.Atoi(l1[4]); err != nil {
		return st, fmt.Errorf("zebra: host status formats-in-buffer %q: %w", l1[4], err)
	}
	if st.LabelsRemaining, err = strconv.Atoi(l2[8]); err != nil {
		return st, fmt.Errorf("zebra: host status labels-remaining %q: %w", l2[8], err)
	}
	st.PaperOut = l1[1] == "1"
	st.Paused = l1[2] == "1"
	st.BufferFull = l1[5] == "1"
	st.PartialFormat = l1[7] == "1"
	st.HeadOpen = l2[2] == "1"
	st.RibbonOut = l2[3] == "1"
	return st, nil
}

func splitStatusLine(line string) []string {
	fields := strings.Split(strings.Trim(line, " \x02\x03"), ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}
