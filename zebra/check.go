package zebra

import (
	"context"
	"fmt"
	"net"
	"strings"
)

// Report is the result of a pre-flight check.
type Report struct {
	Info     Info
	Firmware string // version of the image being checked
	// Problems block the upgrade.
	Problems []string
	// Warnings do not block the upgrade but deserve the operator's attention.
	Warnings []string
	// UpToDate is true when the printer already runs the image's version.
	UpToDate bool
}

// OK reports whether the upgrade may proceed.
func (r *Report) OK() bool { return len(r.Problems) == 0 }

// Check queries the printer and decides whether fw can be installed on it
// with default UpgradeOptions. A non-nil error means the printer could not be
// queried; a failed check is reported through Report.Problems.
func (p *Printer) Check(ctx context.Context, fw *Firmware) (*Report, error) {
	return p.CheckWith(ctx, fw, UpgradeOptions{})
}

// CheckWith is Check for an upgrade that would run with opts.
func (p *Printer) CheckWith(ctx context.Context, fw *Firmware, opts UpgradeOptions) (*Report, error) {
	info, err := p.Info(ctx)
	if err != nil {
		return nil, err
	}
	r := &Report{Info: info, Firmware: fw.Version()}

	if !versionRe.MatchString(info.Firmware) {
		// Special builds (e.g. SP93-006444A) don't name their branch, but
		// appl.version does: "9321" is branch V93, build series 21.
		series, _, _ := strings.Cut(strings.TrimPrefix(fw.Version(), fw.Branch()+"."), ".")
		platform := strings.TrimPrefix(fw.Branch(), "V") + series
		switch {
		case info.ApplVersion != platform:
			r.Problems = append(r.Problems, fmt.Sprintf(
				"printer runs non-standard firmware %s on platform %s, which does not match %s",
				info.Firmware, orUnknown(info.ApplVersion), fw.Version()))
		case !opts.ReplaceSpecialBuild:
			r.Problems = append(r.Problems, fmt.Sprintf(
				"printer runs non-standard firmware %s (a Zebra special build?); installing %s would replace it",
				info.Firmware, fw.Version()))
		default:
			r.Warnings = append(r.Warnings, fmt.Sprintf(
				"replacing special build %s with standard %s; it cannot be reinstalled from public downloads",
				info.Firmware, fw.Version()))
		}
	} else if info.Branch() != fw.Branch() {
		r.Problems = append(r.Problems, fmt.Sprintf(
			"firmware branch %s does not match printer branch %s (%s runs %s)",
			fw.Branch(), info.Branch(), info.Model, info.Firmware))
	}
	if !info.AllowFirmwareDownloads {
		r.Problems = append(r.Problems, "printer has device.allow_firmware_downloads set to \"no\"")
	}
	if hs := info.HostStatus; hs.Busy() {
		r.Problems = append(r.Problems, fmt.Sprintf(
			"printer is busy (%d formats queued, %d labels remaining)", hs.FormatsInBuffer, hs.LabelsRemaining))
	}

	r.UpToDate = info.Firmware == fw.Version()
	if c, ok := compareVersions(fw.Version(), info.Firmware); ok && c < 0 && info.Branch() == fw.Branch() {
		r.Warnings = append(r.Warnings, fmt.Sprintf("this is a downgrade: %s → %s", info.Firmware, fw.Version()))
	}
	if info.HostStatus.Paused {
		r.Warnings = append(r.Warnings, "printer is paused")
	}
	if info.HostStatus.HeadOpen {
		r.Warnings = append(r.Warnings, "printhead is open")
	}
	if info.Hostname == "" && net.ParseIP(p.Host) == nil && !r.UpToDate {
		r.Warnings = append(r.Warnings, fmt.Sprintf(
			"printer runs Link-OS %s; upgrading past 7.6 migrates its network hostname, so %q may not resolve after reboot — consider using its IP address",
			orUnknown(info.LinkOSVersion), p.Host))
	}
	return r, nil
}

func orUnknown(s string) string {
	if s == "" {
		return "(unknown)"
	}
	return s
}

// CheckError is returned by Upgrade when the pre-flight check fails.
type CheckError struct {
	Report *Report
}

func (e *CheckError) Error() string {
	return "zebra: pre-flight failed: " + strings.Join(e.Report.Problems, "; ")
}
