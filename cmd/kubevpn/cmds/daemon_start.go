package cmds

import (
	"errors"
	"net/http"
	"os"

	"github.com/spf13/cobra"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	"github.com/wencaiwulue/kubevpn/pkg/daemon"
	"github.com/wencaiwulue/kubevpn/pkg/daemon/action"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

func init() {
	daemonCmd.AddCommand(daemonStartCmd)
}

var daemonStartCmd = &cobra.Command{
	Use:   "start",
	Short: "start",
	Long:  `start`,
	PreRunE: func(cmd *cobra.Command, _ []string) error {
		// already exist socket, check whether it can connect or not
		if _, err := os.Stat(config.DaemonSock); err == nil {
			_, err = action.CallDaemonUptime(cmd.Context())
			if err == nil {
				return errors.New("daemon server already running")
			}
			// if can not connect
			_ = os.Remove(config.DaemonSock)
		}
		if !util.IsAdmin() {
			return errors.New("needs to startup with super privilege")
		}
		go func() { _ = http.ListenAndServe("localhost:6060", nil) }()
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return (&daemon.Options{SockPath: config.DaemonSock}).Start(cmd.Context())
	},
}
