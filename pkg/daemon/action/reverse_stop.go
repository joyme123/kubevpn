package action

import (
	"context"
	"github.com/wencaiwulue/kubevpn/pkg/util"
	"io"
	"os"

	log "github.com/sirupsen/logrus"
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
	Mode           handler.Mode      `json:"mode"`
	Headers        map[string]string `json:"headers"`
	Workloads      []string          `json:"workloads"`
}

func (r *ReverseStopAction) HandleStream(ctx context.Context, resp io.Writer) error {
	writer := io.MultiWriter(log.StandardLogger().Out, resp)
	var logger = &log.Logger{
		Out:       writer,
		Formatter: new(util.Format),
		Hooks:     make(log.LevelHooks),
		Level:     log.InfoLevel,
	}

	var options = handler.ReverseOptions{
		KubeconfigPath: r.KubeconfigPath,
		Namespace:      r.Namespace,
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

	current.Cleanup()
	current = nil
	return nil
}

func init() {
	daemon.HandlerMap[ReverseStop] = &ReverseStopAction{}
}
