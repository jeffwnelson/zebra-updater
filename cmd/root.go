package cmd

import (
	"os"
	"time"

	"github.com/spf13/cobra"
)

var (
	flagPort           int
	flagConnectTimeout time.Duration
	flagRebootTimeout  time.Duration
)

var rootCmd = &cobra.Command{
	Use:   "zfw",
	Short: "Upgrade Zebra printer firmware over the network",
}

func init() {
	rootCmd.PersistentFlags().IntVar(&flagPort, "port", 9100, "printer TCP port")
	rootCmd.PersistentFlags().DurationVar(&flagConnectTimeout, "connect-timeout", 5*time.Second, "TCP connect timeout")
	rootCmd.PersistentFlags().DurationVar(&flagRebootTimeout, "reboot-timeout", 5*time.Minute, "max time to wait for printer reboot")
	rootCmd.AddCommand(infoCmd)
	rootCmd.AddCommand(upgradeCmd)
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
