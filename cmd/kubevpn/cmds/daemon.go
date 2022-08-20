package cmds

import (
	"github.com/spf13/cobra"
)

func init() {
	RootCmd.AddCommand(daemonCmd)
}

var daemonCmd = &cobra.Command{
	Use:   "daemon",
	Short: "daemon process",
	Long:  `daemon process`,
}
