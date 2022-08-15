package action

import (
	"context"
	"io"
	"os"

	log "github.com/sirupsen/logrus"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

const ConnectInfo = "ConnectInfo"

func CallConnectInfo(ctx context.Context) error {
	var a ConnectInfoAction
	a.Action = ConnectInfo
	return daemon.GetClient().SendStreamRequest(ctx, a, func(reader io.Reader) error {
		_, err := io.Copy(os.Stdout, reader)
		return err
	})
}

type ConnectInfoAction struct {
	daemon.StreamHandler
	daemon.CommonAction
}

func (receiver *ConnectInfoAction) HandleStream(ctx context.Context, resp io.Writer) (err error) {
	var logger = &log.Logger{
		Out:       resp,
		Formatter: new(util.Format),
		Hooks:     make(log.LevelHooks),
		Level:     log.DebugLevel,
	}

	// not connected yet
	if current == nil {
		logger.Infoln("no connected yet")
		return nil
	}

	logger.Println("")

	return nil
}

func init() {
	daemon.HandlerMap[ConnectInfo] = &ConnectInfoAction{}
}
