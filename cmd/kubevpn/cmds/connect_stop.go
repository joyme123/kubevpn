package cmds

import (
	"github.com/spf13/cobra"
	"github.com/wencaiwulue/kubevpn/pkg/daemon/action"
	"github.com/wencaiwulue/kubevpn/pkg/util"
	"os"
	"os/exec"
	"syscall"
)

func init() {
	connectCmd.AddCommand(connectStopCmd)
}

var connectStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "stop connect from remote cluster",
	Long:  `stop connect, and quit all reverse resources`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := action.CallDaemonUptime(cmd.Context())
		if err != nil {
			if !util.IsAdmin() {
				return nil
			}
			command := exec.Command("kubevpn", "daemon", "start")
			command.SysProcAttr = func() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }()
			err := command.Start()
			if err != nil {
				return err
			}
			go command.Wait()

			// wait for daemon server started up fully
			for cmd.Context().Err() == nil {
				_, err = action.CallDaemonUptime(cmd.Context())
				if err == nil {
					break
				}
			}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		return action.CallConnectStop(cmd.Context(), os.Stdout)
	},
}
