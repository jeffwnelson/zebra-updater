package cmd

import (
	"fmt"
	"os"

	"github.com/jeffwnelson/zebra-updater/internal/printer"
	"github.com/spf13/cobra"
)

var infoCmd = &cobra.Command{
	Use:   "info <host>",
	Short: "Show printer model, firmware, free flash, and host status",
	Args:  cobra.ExactArgs(1),
	Run: func(_ *cobra.Command, args []string) {
		host := args[0]
		p := &printer.Printer{Host: host, Port: flagPort, Timeout: flagConnectTimeout}

		fields := []struct{ label, varName string }{
			{"Model         ", "device.product.name"},
			{"Firmware      ", "appl.name"},
			{"Free flash    ", "memory.flash_free"},
			{"Host status   ", "device.host_status"},
			{"IP address    ", "ip.addr"},
		}
		for _, fld := range fields {
			val, err := p.GetVar(fld.varName)
			if err != nil {
				fmt.Fprintf(os.Stderr, "error querying %s: %v\n", fld.varName, err)
				os.Exit(1)
			}
			fmt.Printf("%s %s\n", fld.label, val)
		}
	},
}
