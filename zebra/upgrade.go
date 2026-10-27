package zebra

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrUpToDate is returned by Upgrade when the printer already runs the
// image's version and UpgradeOptions.Force is not set.
var ErrUpToDate = errors.New("zebra: printer already runs this firmware version")

// Phase identifies a stage of an upgrade.
type Phase int

const (
	PhaseSending   Phase = iota // streaming the image to the printer
	PhaseRebooting              // waiting for the printer to flash and restart
	PhaseDone                   // printer reports the new version
)

func (ph Phase) String() string {
	switch ph {
	case PhaseSending:
		return "sending"
	case PhaseRebooting:
		return "rebooting"
	case PhaseDone:
		return "done"
	}
	return fmt.Sprintf("Phase(%d)", int(ph))
}

// Progress is reported to UpgradeOptions.Progress as the upgrade advances.
type Progress struct {
	Phase Phase
	Sent  int64 // bytes written so far (PhaseSending)
	Total int64 // image size
	// Offline is true once the printer has been seen unreachable during
	// PhaseRebooting.
	Offline bool
}

// UpgradeOptions configures Upgrade.
type UpgradeOptions struct {
	// Force reinstalls the image even if the printer already runs it.
	Force bool
	// Progress, if set, is called from the upgrading goroutine. During
	// PhaseSending it is called after every write.
	Progress func(Progress)
}

// TransferError is returned when the image could not be fully sent. The
// printer may be left mid-download; inspect it before retrying.
type TransferError struct {
	Sent, Total int64
	Err         error
}

func (e *TransferError) Error() string {
	return fmt.Sprintf("zebra: transfer failed after %d of %d bytes: %v", e.Sent, e.Total, e.Err)
}

func (e *TransferError) Unwrap() error { return e.Err }

// RebootError is returned when the image was sent but the printer did not
// report the new version before the reboot timeout.
type RebootError struct {
	// SawOffline is true if the printer was unreachable at some point after
	// the transfer, i.e. it most likely started flashing.
	SawOffline bool
	// LastVersion is the last version the printer reported, if any.
	LastVersion string
	Err         error
}

func (e *RebootError) Error() string {
	if !e.SawOffline {
		return fmt.Sprintf("zebra: printer never went offline after the transfer (still reports %q): %v", e.LastVersion, e.Err)
	}
	return fmt.Sprintf("zebra: printer did not come back on the new firmware: %v", e.Err)
}

func (e *RebootError) Unwrap() error { return e.Err }

// VersionMismatchError is returned when the printer rebooted but reports a
// version other than the one sent, which usually means it rejected the image.
type VersionMismatchError struct {
	Want, Got string
}

func (e *VersionMismatchError) Error() string {
	return fmt.Sprintf("zebra: printer came back on %s, want %s", e.Got, e.Want)
}

// Upgrade installs fw on the printer. It re-runs Check first and returns a
// *CheckError if any problem is found, or ErrUpToDate if there is nothing to
// do. After sending the image it waits for the printer to reboot and report
// fw's version.
//
// Errors after the transfer has begun are *TransferError, *RebootError, or
// *VersionMismatchError.
func (p *Printer) Upgrade(ctx context.Context, fw *Firmware, opts UpgradeOptions) error {
	report, err := p.Check(ctx, fw)
	if err != nil {
		return err
	}
	if !report.OK() {
		return &CheckError{Report: report}
	}
	if report.UpToDate && !opts.Force {
		return ErrUpToDate
	}

	progress := opts.Progress
	if progress == nil {
		progress = func(Progress) {}
	}
	if err := p.send(ctx, fw, progress); err != nil {
		return err
	}
	if err := p.awaitVersion(ctx, fw, progress); err != nil {
		return err
	}
	progress(Progress{Phase: PhaseDone, Sent: fw.Size(), Total: fw.Size()})
	return nil
}

func (p *Printer) send(ctx context.Context, fw *Firmware, progress func(Progress)) error {
	total := fw.Size()
	conn, err := p.dial(ctx)
	if err != nil {
		return &TransferError{Total: total, Err: err}
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()

	stall := orDefault(p.WriteStallTimeout, DefaultWriteStallTimeout)
	r := fw.reader()
	buf := make([]byte, 32*1024)
	var sent int64
	progress(Progress{Phase: PhaseSending, Total: total})
	for sent < total {
		n, rerr := r.Read(buf)
		if n > 0 {
			conn.SetWriteDeadline(time.Now().Add(stall))
			if _, werr := conn.Write(buf[:n]); werr != nil {
				if ctx.Err() != nil {
					werr = ctx.Err()
				}
				return &TransferError{Sent: sent, Total: total, Err: werr}
			}
			sent += int64(n)
			progress(Progress{Phase: PhaseSending, Sent: sent, Total: total})
		}
		if rerr != nil && sent < total {
			return &TransferError{Sent: sent, Total: total, Err: fmt.Errorf("read firmware: %w", rerr)}
		}
	}
	return nil
}

// awaitVersion polls until the printer reports fw's version or the reboot
// timeout expires. The printer keeps answering on its old firmware while it
// processes the image, and may restart more than once, so an old version is
// only judged a mismatch at the deadline, and only if the printer went
// offline and came back on it.
func (p *Printer) awaitVersion(ctx context.Context, fw *Firmware, progress func(Progress)) error {
	ctx, cancel := context.WithTimeout(ctx, orDefault(p.RebootTimeout, DefaultRebootTimeout))
	defer cancel()
	interval := orDefault(p.PollInterval, DefaultPollInterval)

	var sawOffline, backOnOld bool
	var last string
	var lastErr error
	progress(Progress{Phase: PhaseRebooting, Sent: fw.Size(), Total: fw.Size()})
	for {
		v, err := p.GetVar(ctx, "appl.name")
		if err == nil {
			if v == fw.Version() {
				return nil
			}
			last, backOnOld = v, sawOffline
		} else {
			lastErr = err
			if !sawOffline && ctx.Err() == nil && p.Ping(ctx) != nil {
				sawOffline = true
				progress(Progress{Phase: PhaseRebooting, Sent: fw.Size(), Total: fw.Size(), Offline: true})
			}
		}

		select {
		case <-ctx.Done():
			if backOnOld {
				return &VersionMismatchError{Want: fw.Version(), Got: last}
			}
			if lastErr == nil {
				lastErr = ctx.Err()
			}
			return &RebootError{SawOffline: sawOffline, LastVersion: last, Err: lastErr}
		case <-time.After(interval):
		}
	}
}
