package cmds

import (
	"github.com/spf13/cobra"
	"github.com/wencaiwulue/kubevpn/pkg/daemon/action"
)

func init() {
	daemonCmd.AddCommand(logsCmd)
}

var logsCmd = &cobra.Command{
	Use:   "logs",
	Short: "logs for daemon process",
	Long:  `logs for daemon process`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return action.CallDaemonLogs(cmd.Context())
	},
}
