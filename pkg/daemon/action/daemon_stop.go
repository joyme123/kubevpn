package action

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/util/retry"

	"github.com/wencaiwulue/kubevpn/pkg/daemon"
	"github.com/wencaiwulue/kubevpn/pkg/driver"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

const DaemonStop = "DaemonStop"

func CallDaemonStop(ctx context.Context) error {
	var m = make(map[string]string)
	var s DaemonStopAction
	s.Action = DaemonStop
	err := daemon.GetClient().SendJsonRequest(ctx, s, &m)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil
		}
	}
	return err
}

type DaemonStopAction struct {
	daemon.CommonAction
	daemon.StreamHandler
}

func (a *DaemonStopAction) HandleStream(ctx context.Context, resp io.Writer) (err error) {
	writer := io.MultiWriter(log.StandardLogger().Out, resp)
	var logger = &log.Logger{
		Out:       writer,
		Formatter: new(util.Format),
		Hooks:     make(log.LevelHooks),
		Level:     log.DebugLevel,
	}

	err = CallConnectStop(ctx, writer)
	if err != nil {
		return err
	}
	(&daemon.Options{}).Stop()
	if util.IsWindows() {
		if err := retry.OnError(retry.DefaultRetry, func(err error) bool {
			return err != nil
		}, func() error {
			return driver.UninstallWireGuardTunDriver()
		}); err != nil {
			wd, _ := os.Getwd()
			filename := filepath.Join(wd, "wintun.dll")
			if err = os.Rename(filename, filepath.Join(os.TempDir(), "wintun.dll")); err != nil {
				logger.Warn(err)
			}
		}
	}
	return nil
}

func init() {
	daemon.HandlerMap[DaemonStop] = &DaemonStopAction{}
}
