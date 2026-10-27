package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <host>",
	Short: "Show printer model, firmware, and status",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		info, err := newPrinter(args[0]).Info(context.Background())
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		hs := info.HostStatus
		fmt.Printf("Model           %s\n", info.Model)
		fmt.Printf("Serial          %s\n", info.UniqueID)
		fmt.Printf("Firmware        %s (Link-OS %s)\n", info.Firmware, info.LinkOSVersion)
		fmt.Printf("Hostname        %s\n", orDash(info.Hostname))
		fmt.Printf("FW downloads    %s\n", yesNo(info.AllowFirmwareDownloads))
		fmt.Printf("Busy            %s (%d formats, %d labels remaining)\n", yesNo(hs.Busy()), hs.FormatsInBuffer, hs.LabelsRemaining)
		fmt.Printf("Paused          %s\n", yesNo(hs.Paused))
		fmt.Printf("Head open       %s\n", yesNo(hs.HeadOpen))
		fmt.Printf("Paper out       %s\n", yesNo(hs.PaperOut))
	},
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
