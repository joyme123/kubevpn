package action

import (
	"context"
	"errors"
	log "github.com/sirupsen/logrus"
	"github.com/wencaiwulue/kubevpn/pkg/util"
	"io"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
)

const ConnectStop = "ConnectStop"

func CallConnectStop(ctx context.Context, writer io.Writer) error {
	if writer == nil {
		return errors.New("writer is nil")
	}

	req := ConnectStopAction{}
	req.Action = ConnectStop

	return daemon.GetClient().SendStreamRequest(ctx, req, func(reader io.Reader) error {
		_, err := io.Copy(writer, reader)
		return err
	})
}

type ConnectStopAction struct {
	daemon.StreamHandler
	daemon.CommonAction
}

func (receiver *ConnectStopAction) HandleStream(ctx context.Context, resp io.Writer) (err error) {
	var logger = &log.Logger{
		Out:       resp,
		Formatter: new(util.Format),
		Hooks:     make(log.LevelHooks),
		Level:     log.DebugLevel,
	}
	if current == nil {
		logger.Infoln("not needs to disconnected from cluster")
		return
	}

	connectCancel()

	current.Cleanup()
	current = nil
	logger.Infoln("success disconnect from cluster")
	return
}

func init() {
	daemon.HandlerMap[ConnectStop] = &ConnectStopAction{}
}
