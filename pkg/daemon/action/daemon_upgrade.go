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

const DaemonUpgrade = "DaemonUpgrade"

func CallDaemonUpgrade(ctx context.Context) error {
	var m = make(map[string]string)
	var u DaemonUpgradeAction
	u.Action = DaemonUpgrade

	err := daemon.GetClient().SendJsonRequest(ctx, u, &m)
	if err != nil {
		if strings.Contains(err.Error(), "connection refused") {
			return nil
		}
	}
	return err
}

type DaemonUpgradeAction struct {
	daemon.JsonHandler
	daemon.CommonAction
}

func (receiver DaemonUpgradeAction) HandleJson(ctx context.Context) (interface{}, error) {
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
	daemon.HandlerMap[DaemonUpgrade] = &DaemonUpgradeAction{}
}
