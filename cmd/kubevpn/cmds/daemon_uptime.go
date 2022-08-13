package cmds

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/wencaiwulue/kubevpn/pkg/daemon/action"
)

func init() {
	daemonCmd.AddCommand(daemonUptimeCmd)
}

var daemonUptimeCmd = &cobra.Command{
	Use:   "uptime",
	Short: "uptime",
	Long:  `uptime`,
	RunE: func(cmd *cobra.Command, args []string) error {
		uptime, err := action.CallDaemonUptime(cmd.Context())
		if err != nil {
			if strings.Contains(err.Error(), "connection refused") {
				fmt.Println("daemon not start")
				return nil
			}
			return err
		}
		fmt.Println(time.Now().Sub(time.Unix(uptime, 0)).String())
		return nil
	},
}
