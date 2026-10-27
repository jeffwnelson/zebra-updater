package cmd

import (
	"os"
	"time"

	"github.com/jeffwnelson/zebra-updater/zebra"
	"github.com/spf13/cobra"
)

var (
	flagPort           int
	flagConnectTimeout time.Duration
	flagWriteTimeout   time.Duration
	flagRebootTimeout  time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "zfw",
	Short: "Upgrade Zebra printer firmware over the network",
}

func init() {
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", zebra.DefaultPort, "printer TCP port")
	rootCmd.PersistentFlags().DurationVar(&flagConnectTimeout, "connect-timeout", zebra.DefaultDialTimeout, "TCP connect timeout")
	rootCmd.PersistentFlags().DurationVar(&flagWriteTimeout, "write-timeout", zebra.DefaultWriteStallTimeout, "abandon the transfer if a write blocks this long")
	rootCmd.PersistentFlags().DurationVar(&flagRebootTimeout, "reboot-timeout", zebra.DefaultRebootTimeout, "max time to wait for the printer to reboot on the new firmware")
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(upgradeCmd)
}

func newPrinter(host string) *zebra.Printer {
	return &zebra.Printer{
		Host:              host,
		Port:              flagPort,
		DialTimeout:       flagConnectTimeout,
		WriteStallTimeout: flagWriteTimeout,
		RebootTimeout:     flagRebootTimeout,
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
