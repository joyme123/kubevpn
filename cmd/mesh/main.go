package main

import (
	"flag"
	"os/exec"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"

	"github.com/wencaiwulue/kubevpn/pkg/util"
)

var (
	logger                 *log.Logger
	watchDirectoryFileName string
	port                   uint = 9002
)

func init() {
	logger = log.New()
	log.SetLevel(log.DebugLevel)
	log.SetReportCaller(true)
	log.SetFormatter(&util.Format{})
	flag.StringVar(&watchDirectoryFileName, "watchDirectoryFileName", "/etc/envoy/envoy-config.yaml", "full path to directory to watch for files")
	flag.Parse()
}

func main() {
	go func() {
		command := exec.Command("kubevpn", "serve")
		command.SysProcAttr = func() *syscall.SysProcAttr {
			return &syscall.SysProcAttr{Setsid: true}
		}()
		command.Start()
		go command.Wait()
	}()
	time.Sleep(time.Second * 1)
}
