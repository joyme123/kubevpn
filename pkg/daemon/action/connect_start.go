package action

import (
	"context"
	"errors"
	"io"

	log "github.com/sirupsen/logrus"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
	"github.com/wencaiwulue/kubevpn/pkg/driver"
	"github.com/wencaiwulue/kubevpn/pkg/handler"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

const ConnectStart = "ConnectStart"

var current *handler.ConnectOptions

var connectCtx context.Context
var connectCancel context.CancelFunc

func CallConnectStart(ctx context.Context, param ConnectStartAction, writer io.Writer) error {
	if writer == nil {
		return errors.New("writer is nil")
	}

	param.Action = ConnectStart

	return daemon.GetClient().SendStreamRequest(ctx, param, func(reader io.Reader) error {
		_, err := io.Copy(writer, reader)
		return err
	})
}

type ConnectStartAction struct {
	daemon.StreamHandler
	daemon.CommonAction

	KubeconfigPath string `json:"kubeconfigPath"`
	Namespace      string `json:"namespace"`
}

func (receiver *ConnectStartAction) HandleStream(ctx context.Context, resp io.Writer) (err error) {
	var logger = &log.Logger{
		Out:       io.MultiWriter(log.StandardLogger().Out, resp),
		Formatter: new(util.Format),
		Hooks:     make(log.LevelHooks),
		Level:     log.DebugLevel,
	}
	var temp = &handler.ConnectOptions{
		KubeconfigPath: receiver.KubeconfigPath,
		Namespace:      receiver.Namespace,
	}
	err = temp.InitClient()
	if err != nil {
		return err
	}
	logger.Infof("kubeconfig path: %s, namespace: %s", receiver.KubeconfigPath, receiver.Namespace)

	// already Connect to this namespace
	if current != nil && current.NamespaceID == temp.NamespaceID {
		logger.Infoln("already connected")
		return nil
	}

	// switch to another ns
	if current != nil && current.NamespaceID != temp.NamespaceID {
		// DaemonStop current
		err = CallConnectStop(ctx, resp)
		if err != nil {
			logger.Infoln("failed to disconnect from cluster")
			return err
		}
	}

	// not connected yet
	current = temp
	connectCtx, connectCancel = context.WithCancel(context.Background())

	defer func() {
		if err != nil {
			current.Cleanup()
			// cleanup
			current = nil
		}
	}()

	if util.IsWindows() {
		driver.InstallWireGuardTunDriver(logger)
	}

	err = current.DoConnect(ctx, connectCtx, logger)
	if err != nil {
		return err
	}
	logger.Infoln("---------------------------------------------------------------------------")
	logger.Infoln("   Now you can access resources in the kubernetes cluster, enjoy it :)     ")
	logger.Infoln("---------------------------------------------------------------------------")
	return nil
}

func init() {
	daemon.HandlerMap[ConnectStart] = &ConnectStartAction{}
}
