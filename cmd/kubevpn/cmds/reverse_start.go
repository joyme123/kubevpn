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

var reverseOptions = handler.ReverseOptions{}

func init() {
	reverseStartCmd.Flags().StringVar(&reverseOptions.KubeconfigPath, "kubeconfig", clientcmd.RecommendedHomeFile, "kubeconfig")
	reverseStartCmd.Flags().StringVarP(&reverseOptions.Namespace, "namespace", "n", "", "namespace")
	reverseStartCmd.PersistentFlags().StringArrayVar(&reverseOptions.Workloads, "workloads", []string{}, "workloads, like: pods/tomcat, deployment/nginx, replicaset/tomcat...")
	reverseStartCmd.Flags().StringVar((*string)(&reverseOptions.Mode), "mode", string(handler.Reverse), "default mode is reverse")
	reverseStartCmd.Flags().StringToStringVarP(&reverseOptions.Headers, "headers", "H", map[string]string{}, "headers, format is k=v, like: k1=v1,k2=v2")
	reverseStartCmd.Flags().BoolVar(&config.Debug, "debug", false, "true/false")
	reverseCmd.AddCommand(reverseStartCmd)
}

var reverseStartCmd = &cobra.Command{
	Use:   "start",
	Short: "start reverse remote resource traffic to local machine",
	Long:  `reverse remote traffic to local machine`,
	PreRunE: func(cmd *cobra.Command, args []string) (err error) {
		_, err = action.CallDaemonUptime(cmd.Context())
		if err != nil {
			if !util.IsAdmin() {
				util.RunWithElevated()
				os.Exit(0)
			}
			command := exec.Command("kubevpn", "daemon", "start")
			command.SysProcAttr = func() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }()
			err = command.Start()
			if err != nil {
				return
			}
			go command.Wait()
		}
		return
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := reverseOptions.InitClient()
		if err != nil {
			return err
		}
		return action.CallReverseStart(cmd.Context(), action.ReverseStartAction{
			KubeconfigPath: reverseOptions.KubeconfigPath,
			Namespace:      reverseOptions.Namespace,
			Mode:           reverseOptions.Mode,
			Headers:        reverseOptions.Headers,
			Workloads:      reverseOptions.Workloads,
		})
	},
}
