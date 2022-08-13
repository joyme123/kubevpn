package action

import (
	"bufio"
	"context"
	"io"

	log "github.com/sirupsen/logrus"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
)

const DaemonLogs = "DaemonLogs"

func CallDaemonLogs(ctx context.Context) error {
	var s DaemonLogsAction
	s.Action = DaemonLogs

	err := daemon.GetClient().SendStreamRequest(ctx, s, func(r io.Reader) error {
		reader := bufio.NewReader(r)
		for ctx.Err() == nil {
			line, _, err := reader.ReadLine()
			if err != nil {
				return err
			}
			println(string(line))
		}
		return nil
	})
	//if err != nil {
	//	if strings.Contains(err.Error(), "connection refused") {
	//		return nil
	//	}
	//}
	return err
}

type DaemonLogsAction struct {
	daemon.CommonAction
	daemon.StreamHandler
}

func (receiver DaemonLogsAction) HandleStream(ctx context.Context, resp io.Writer) (err error) {
	backup := log.StandardLogger().Out
	defer log.SetOutput(backup)
	log.SetOutput(io.MultiWriter(resp, backup))
	<-ctx.Done()
	return
}

func init() {
	daemon.HandlerMap[DaemonLogs] = &DaemonLogsAction{}
}
