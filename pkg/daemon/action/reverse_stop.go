package action

import (
	"context"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/types"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
	"github.com/wencaiwulue/kubevpn/pkg/handler"
)

const ReverseStop = "ReverseStop"

func CallReverseStop(ctx context.Context, r ReverseStartAction) error {
	r.Action = ReverseStop

	return daemon.GetClient().SendStreamRequest(ctx, r, func(reader io.Reader) error {
		_, err := io.Copy(os.Stdout, reader)
		return err
	})
}

type ReverseStopAction struct {
	daemon.StreamHandler
	daemon.CommonAction

	KubeconfigPath string            `json:"kubeconfigPath"`
	Namespace      string            `json:"namespace"`
	NamespaceID    types.UID         `json:"namespaceID"`
	Mode           handler.Mode      `json:"mode"`
	Headers        map[string]string `json:"headers"`
	Workloads      []string          `json:"workloads"`
}

func (r *ReverseStopAction) HandleStream(ctx context.Context, resp io.Writer) error {
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

	if current == nil {
		logger.Infoln("not connect to cluster")
		return nil
	}

	options.LocalTunIP = current.LocalTunIP
	options.PreCheckResource()

	handler.Cleanup(current.GetDHCP().Release, current.GetClient(), current.GetNamespace())
	current = nil
	return nil
}

func init() {
	daemon.HandlerMap[ReverseStop] = &ReverseStopAction{}
}
