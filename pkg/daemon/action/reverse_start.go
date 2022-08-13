package action

import (
	"context"
	"fmt"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
	"github.com/wencaiwulue/kubevpn/pkg/handler"
)

const ReverseStart = "ReverseStart"

func CallReverseStart(ctx context.Context, r ReverseStartAction) error {
	r.Action = ReverseStart

	return daemon.GetClient().SendStreamRequest(ctx, r, func(reader io.Reader) error {
		_, err := io.Copy(os.Stdout, reader)
		return err
	})
}

type ReverseStartAction struct {
	daemon.StreamHandler
	daemon.CommonAction

	KubeconfigPath string            `json:"kubeconfigPath"`
	Namespace      string            `json:"namespace"`
	NamespaceID    types.UID         `json:"namespaceID"`
	Mode           handler.Mode      `json:"mode"`
	Headers        map[string]string `json:"headers"`
	Workloads      []string          `json:"workloads"`
}

func (r *ReverseStartAction) HandleStream(ctx context.Context, resp io.Writer) error {
	writer := io.MultiWriter(log.StandardLogger().Out, resp)
	var logger = &log.Logger{
		Out:       writer,
		Formatter: new(log.TextFormatter),
		Hooks:     make(log.LevelHooks),
		Level:     log.DebugLevel,
	}

	var options = handler.ReverseOptions{
		KubeconfigPath: r.KubeconfigPath,
		Namespace:      r.Namespace,
		NamespaceID:    r.NamespaceID,
		Mode:           r.Mode,
		Headers:        r.Headers,
		Workloads:      r.Workloads,
	}

	err := options.InitClient()
	if err != nil {
		return err
	}

	// needs to Connect to cluster if not connected yet
	if current == nil {
		logger.Infoln("not Connect to cluster, call Connect ...")

		req := ConnectStartAction{
			KubeconfigPath: options.KubeconfigPath,
			Namespace:      options.Namespace,
			NamespaceID:    options.NamespaceID,
		}
		err = CallConnectStart(ctx, req, writer)
		if err != nil {
			logger.Infoln("failed to connect to cluster")
			return err
		}
	}

	if current == nil {
		return fmt.Errorf("connect to cluster failed\n")
	}

	logger.Debugln("connected to cluster")

	options.LocalTunIP = current.LocalTunIP
	options.PreCheckResource()
	logger.Infof("kubeconfig path: %s, namespace: %s, services: %v", r.KubeconfigPath, r.Namespace, r.Workloads)

	err = options.DoReverse(ctx, logger)
	if err != nil {
		handler.Cleanup(options.GetDHCP().Release, options.GetClient(), options.GetNamespace())
		return err
	}
	logger.Println("------------------------------------------------------------------------------------------------------------------")
	logger.Println("    Now you can access resources in the kubernetes cluster, the traffic with ReverseStart to your local computer :)    ")
	logger.Println("------------------------------------------------------------------------------------------------------------------")
	return nil
}

func init() {
	daemon.HandlerMap[ReverseStart] = &ReverseStartAction{}
}
