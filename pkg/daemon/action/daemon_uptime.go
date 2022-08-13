package action

import (
	"context"
	"io"
	"strconv"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
)

const DaemonUptime = "DaemonUptime"

func CallDaemonUptime(ctx context.Context) (int64, error) {
	var timestamp int64

	var u DaemonUptimeAction
	u.Action = DaemonUptime

	err := daemon.GetClient().SendStreamRequest(ctx, u, func(reader io.Reader) error {
		all, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		i64, err := strconv.ParseInt(string(all), 10, 64)
		if err != nil {
			return err
		}
		timestamp = i64
		return nil
	})
	if err != nil {
		return 0, err
	}
	return timestamp, nil
}

type DaemonUptimeAction struct {
	daemon.StreamHandler
	daemon.CommonAction
}

func (receiver DaemonUptimeAction) HandleStream(ctx context.Context, resp io.Writer) error {
	formatInt := strconv.FormatInt(daemon.Uptime, 10)
	_, err := resp.Write([]byte(formatInt))
	return err
}

func init() {
	daemon.HandlerMap[DaemonUptime] = &DaemonUptimeAction{}
}
