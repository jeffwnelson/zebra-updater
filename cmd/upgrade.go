package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jeffwnelson/zebra-updater/internal/firmware"
	"github.com/jeffwnelson/zebra-updater/internal/printer"
	"github.com/spf13/cobra"
)

const (
	exitPreflight = 2
	exitTransfer  = 3
	exitReboot    = 4
)

var (
	flagDryRun        bool
	flagYes           bool
	flagForce         bool
	flagTargetModel   string
	flagTargetVersion string
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade <host> <firmware.zpl>",
	Short: "Upgrade firmware on a Zebra printer",
	Args:  cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		host, fwPath := args[0], args[1]
		runUpgrade(host, fwPath)
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "run pre-flight only, do not send firmware")
	upgradeCmd.Flags().BoolVar(&flagYes, "yes", false, "skip confirmation prompt")
	upgradeCmd.Flags().BoolVar(&flagForce, "force", false, "upgrade even if firmware version matches")
	upgradeCmd.Flags().StringVar(&flagTargetModel, "target-model", "", "override model declared by firmware header (fallback if parser cannot extract)")
	upgradeCmd.Flags().StringVar(&flagTargetVersion, "target-version", "", "override version declared by firmware header")
}

func runUpgrade(host, fwPath string) {
	fw, err := firmware.Open(fwPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open firmware: %v\n", err)
		os.Exit(exitPreflight)
	}
	defer fw.Close()

	targetModel := fw.Model()
	if flagTargetModel != "" {
		targetModel = flagTargetModel
	}
	targetVersion := fw.Version()
	if flagTargetVersion != "" {
		targetVersion = flagTargetVersion
	}
	if targetModel == "" || targetVersion == "" {
		fmt.Fprintln(os.Stderr, "pre-flight: firmware header could not be parsed; provide --target-model and --target-version")
		os.Exit(exitPreflight)
	}

	p := &printer.Printer{Host: host, Port: flagPort, Timeout: flagConnectTimeout}

	if err := p.Ping(); err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: cannot reach %s:%d: %v\n", host, flagPort, err)
		os.Exit(exitPreflight)
	}

	model, err := p.GetVar("device.product.name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query model: %v\n", err)
		os.Exit(exitPreflight)
	}
	if !strings.Contains(model, targetModel) {
		fmt.Fprintf(os.Stderr, "pre-flight: model mismatch — file targets %s, printer reports %s\n", targetModel, model)
		os.Exit(exitPreflight)
	}

	current, err := p.GetVar("appl.name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query version: %v\n", err)
		os.Exit(exitPreflight)
	}
	if current == targetVersion && !flagForce {
		fmt.Printf("already on %s; nothing to do (use --force to reflash)\n", targetVersion)
		os.Exit(0)
	}

	free, err := p.GetVar("memory.flash_free")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query free flash: %v\n", err)
		os.Exit(exitPreflight)
	}
	freeBytes, err := strconv.ParseInt(strings.TrimSpace(free), 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: unparseable memory.flash_free=%q: %v\n", free, err)
		os.Exit(exitPreflight)
	}
	need := int64(float64(fw.Size()) * 1.2)
	if freeBytes < need {
		fmt.Fprintf(os.Stderr, "pre-flight: low flash — have %d bytes, need ≥ %d\n", freeBytes, need)
		os.Exit(exitPreflight)
	}

	status, err := p.GetVar("device.host_status")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pre-flight: query host status: %v\n", err)
		os.Exit(exitPreflight)
	}
	// The first whitespace-separated field of host_status is the number of formats
	// in receive buffer; a nonzero value means the printer is busy. We treat any
	// non-"0" first token as busy.
	if first := strings.Fields(status); len(first) > 0 && first[0] != "0" {
		fmt.Fprintf(os.Stderr, "pre-flight: printer busy (host_status=%q)\n", status)
		os.Exit(exitPreflight)
	}

	fmt.Printf("Pre-flight OK: %s — %s → %s, %d bytes\n", model, current, targetVersion, fw.Size())

	if flagDryRun {
		return
	}

	if !flagYes {
		fmt.Print("Proceed with upgrade? [y/N] ")
		reader := bufio.NewReader(os.Stdin)
		ans, _ := reader.ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("aborted")
			return
		}
	}

	if err := pushFirmware(p, fw); err != nil {
		fmt.Fprintf(os.Stderr, "transfer failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "WARNING: printer may be in a partially flashed state — run `zfw info` to inspect before retrying")
		os.Exit(exitTransfer)
	}

	if err := waitForReboot(p, flagRebootTimeout); err != nil {
		fmt.Fprintf(os.Stderr, "reboot wait: %v\n", err)
		fmt.Fprintln(os.Stderr, "Firmware was sent. Power-cycle the printer and run `zfw info` to confirm state.")
		os.Exit(exitReboot)
	}

	after, err := p.GetVar("appl.name")
	if err != nil {
		fmt.Fprintf(os.Stderr, "post-reboot version query failed: %v\n", err)
		os.Exit(exitReboot)
	}
	fmt.Printf("Upgrade complete: %s → %s\n", current, after)
	if after != targetVersion {
		fmt.Fprintf(os.Stderr, "WARNING: post-reboot version %q does not match target %q\n", after, targetVersion)
		os.Exit(exitReboot)
	}
}

func pushFirmware(p *printer.Printer, fw *firmware.File) error {
	total := fw.Size()
	last := time.Now()
	return p.Send(fw.Reader(), func(written int64) {
		if time.Since(last) < 500*time.Millisecond && written < total {
			return
		}
		last = time.Now()
		fmt.Printf("\r  %.1f / %.1f MiB", float64(written)/(1<<20), float64(total)/(1<<20))
		if written >= total {
			fmt.Println()
		}
	})
}

// waitForReboot polls the printer's TCP port until we observe a "down" sample
// followed by an "up" sample, indicating the flash cycle completed.
func waitForReboot(p *printer.Printer, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	sawDown := false
	for time.Now().Before(deadline) {
		err := p.Ping()
		if err != nil && !sawDown {
			fmt.Println("Printer is flashing...")
			sawDown = true
		}
		if err == nil && sawDown {
			fmt.Println("Printer is back online.")
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	if !sawDown {
		return fmt.Errorf("printer never went offline within %s; flash may not have started", timeout)
	}
	return fmt.Errorf("printer did not come back online within %s", timeout)
}
