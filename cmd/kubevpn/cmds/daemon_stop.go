package cmds

import (
	"github.com/spf13/cobra"
	
	"github.com/wencaiwulue/kubevpn/pkg/daemon/action"
)

func init() {
	daemonCmd.AddCommand(daemonStopCmd)
}

var daemonStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "stop",
	Long:  `stop`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return action.CallDaemonStop(cmd.Context())
	},
}
