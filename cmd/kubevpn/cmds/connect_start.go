package cmds

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	"github.com/wencaiwulue/kubevpn/pkg/daemon/action"
	"github.com/wencaiwulue/kubevpn/pkg/handler"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

var connect = handler.ConnectOptions{}

func init() {
	connectStartCmd.Flags().StringVar(&connect.KubeconfigPath, "kubeconfig", clientcmd.RecommendedHomeFile, "kubeconfig")
	connectStartCmd.Flags().StringVarP(&connect.Namespace, "namespace", "n", "", "namespace")
	connectStartCmd.Flags().BoolVar(&config.Debug, "debug", false, "true/false")
	connectCmd.AddCommand(connectStartCmd)
}

var connectStartCmd = &cobra.Command{
	Use:   "start",
	Short: "start to connect to remote cluster",
	Long:  `start to connect to remote cluster`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := action.CallDaemonUptime(cmd.Context())
		if err != nil {
			if !util.IsAdmin() {
				util.RunWithElevated()
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
		err := connect.InitClient()
		if err != nil {
			return err
		}
		req := action.ConnectStartAction{
			KubeconfigPath: connect.KubeconfigPath,
			Namespace:      connect.Namespace,
		}
		err = action.CallConnectStart(cmd.Context(), req, os.Stdout)
		return err
	},
}
