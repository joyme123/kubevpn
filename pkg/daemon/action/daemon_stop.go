package action

import (
	"context"
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
	daemon.JsonHandler
}

func (a *DaemonStopAction) HandleJson(ctx context.Context) (interface{}, error) {
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
				log.Warn(err)
			}
		}
	}
	return map[string]string{}, nil
}

func init() {
	daemon.HandlerMap[DaemonStop] = &DaemonStopAction{}
}
