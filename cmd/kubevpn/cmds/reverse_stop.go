package cmds

import (
	"context"
	"errors"
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

var reverseStopOptions = handler.ReverseOptions{}

func init() {
	reverseStopCmd.Flags().StringVar(&reverseStopOptions.KubeconfigPath, "kubeconfig", clientcmd.RecommendedHomeFile, "kubeconfig")
	reverseStopCmd.Flags().StringVarP(&reverseStopOptions.Namespace, "namespace", "n", "", "namespace")
	reverseStopCmd.PersistentFlags().StringArrayVar(&reverseStopOptions.Workloads, "workloads", []string{}, "workloads, like: pods/tomcat, deployment/nginx, replicaset/tomcat...")
	reverseStopCmd.Flags().StringVar((*string)(&reverseStopOptions.Mode), "mode", string(handler.Reverse), "default mode is reverse")
	reverseStopCmd.Flags().StringToStringVarP(&reverseStopOptions.Headers, "headers", "H", map[string]string{}, "headers, format is k=v, like: k1=v1,k2=v2")
	reverseStopCmd.Flags().BoolVar(&config.Debug, "debug", false, "true/false")
	reverseCmd.AddCommand(reverseStopCmd)
}

var reverseStopCmd = &cobra.Command{
	Use:   "stop",
	Short: "stop",
	Long:  `stop`,
	PreRunE: func(cmd *cobra.Command, args []string) error {
		_, err := action.CallDaemonUptime(context.Background())
		if err != nil {
			if !util.IsAdmin() {
				return errors.New("daemon not start")
			} else {
				cmd := exec.Command("kubevpn", "daemon", "start")
				cmd.SysProcAttr = func() *syscall.SysProcAttr { return &syscall.SysProcAttr{Setsid: true} }()
				err := cmd.Start()
				if err != nil {
					os.Exit(1)
				}
				go cmd.Wait()
			}
		}
		return nil
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		err := reverseOptions.InitClient()
		if err != nil {
			return err
		}
		id, err := reverseOptions.GetNamespaceId()
		if err != nil {
			return err
		}
		return action.CallReverseStop(cmd.Context(), action.ReverseStartAction{
			KubeconfigPath: reverseOptions.KubeconfigPath,
			Namespace:      reverseOptions.Namespace,
			NamespaceID:    id,
			Mode:           reverseOptions.Mode,
			Headers:        reverseOptions.Headers,
			Workloads:      reverseOptions.Workloads,
		})
	},
}
