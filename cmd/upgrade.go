package cmd

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jeffwnelson/zebra-updater/zebra"
	"github.com/spf13/cobra"
)

const (
	exitPreflight = 2
	exitTransfer  = 3
	exitReboot    = 4
)

var (
	flagDryRun bool
	flagYes    bool
	flagForce  bool

	flagReplaceSpecial bool
)

var upgradeCmd = &cobra.Command{
	Use:   "upgrade <host> <firmware.zpl>",
	Short: "Upgrade firmware on a Zebra printer",
	Args:  cobra.ExactArgs(2),
	Run: func(_ *cobra.Command, args []string) {
		runUpgrade(args[0], args[1])
	},
}

func init() {
	upgradeCmd.Flags().BoolVar(&flagDryRun, "dry-run", false, "run pre-flight only, do not send firmware")
	upgradeCmd.Flags().BoolVar(&flagYes, "yes", false, "skip confirmation prompt")
	upgradeCmd.Flags().BoolVar(&flagForce, "force", false, "reinstall even if the printer already runs this version")
	upgradeCmd.Flags().BoolVar(&flagReplaceSpecial, "replace-special-build", false, "overwrite non-standard (special build) printer firmware with this standard image")
}

func runUpgrade(host, fwPath string) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fw, err := zebra.OpenFirmware(fwPath)
	if err != nil {
		fail(exitPreflight, err)
	}
	defer fw.Close()

	p := newPrinter(host)
	opts := zebra.UpgradeOptions{Force: flagForce, ReplaceSpecialBuild: flagReplaceSpecial, Progress: progressPrinter()}
	report, err := p.CheckWith(ctx, fw, opts)
	if err != nil {
		fail(exitPreflight, err)
	}
	info := report.Info
	fmt.Printf("Printer   %s (%s) on %s, Link-OS %s\n", info.Model, info.UniqueID, info.Firmware, info.LinkOSVersion)
	fmt.Printf("Firmware  %s, %.1f MiB\n", fw.Version(), float64(fw.Size())/(1<<20))
	for _, w := range report.Warnings {
		fmt.Printf("warning: %s\n", w)
	}
	if !report.OK() {
		for _, prob := range report.Problems {
			fmt.Fprintf(os.Stderr, "pre-flight: %s\n", prob)
		}
		os.Exit(exitPreflight)
	}
	if report.UpToDate && !flagForce {
		fmt.Printf("already on %s; nothing to do (use --force to reflash)\n", fw.Version())
		return
	}
	fmt.Println("Pre-flight OK")
	if flagDryRun {
		return
	}

	if !flagYes {
		if st, _ := os.Stdin.Stat(); st.Mode()&os.ModeCharDevice == 0 {
			fail(1, errors.New("stdin is not a terminal; pass --yes to upgrade non-interactively"))
		}
		fmt.Printf("Upgrade %s from %s to %s? [y/N] ", host, info.Firmware, fw.Version())
		ans, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.ToLower(strings.TrimSpace(ans)) != "y" {
			fmt.Println("aborted")
			os.Exit(1)
		}
	}

	err = p.Upgrade(ctx, fw, opts)
	var (
		checkErr    *zebra.CheckError
		transferErr *zebra.TransferError
		rebootErr   *zebra.RebootError
		mismatchErr *zebra.VersionMismatchError
	)
	switch {
	case err == nil:
		fmt.Printf("Upgrade complete: %s → %s\n", info.Firmware, fw.Version())
	case errors.As(err, &checkErr), errors.Is(err, zebra.ErrUpToDate):
		fail(exitPreflight, err)
	case errors.As(err, &transferErr):
		fmt.Println()
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "WARNING: the printer may be stuck mid-download. Power-cycle it and run `zfw info` before retrying.")
		os.Exit(exitTransfer)
	case errors.As(err, &rebootErr), errors.As(err, &mismatchErr):
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, "Firmware was sent. Run `zfw info` to check the printer's state.")
		os.Exit(exitReboot)
	default:
		fail(1, err)
	}
}

func progressPrinter() func(zebra.Progress) {
	var last time.Time
	var phase zebra.Phase = -1
	return func(pr zebra.Progress) {
		switch pr.Phase {
		case zebra.PhaseSending:
			if time.Since(last) < 500*time.Millisecond && pr.Sent < pr.Total {
				return
			}
			last = time.Now()
			fmt.Printf("\r  sent %.1f / %.1f MiB", float64(pr.Sent)/(1<<20), float64(pr.Total)/(1<<20))
			if pr.Sent >= pr.Total {
				fmt.Println()
			}
		case zebra.PhaseRebooting:
			if phase != zebra.PhaseRebooting {
				fmt.Println("Waiting for the printer to flash and reboot...")
			} else if pr.Offline {
				fmt.Println("  printer is offline (flashing)")
			}
		}
		phase = pr.Phase
	}
}

func fail(code int, err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(code)
}
