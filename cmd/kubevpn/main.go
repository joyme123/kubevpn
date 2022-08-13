package main

import (
	"context"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"syscall"

	"github.com/wencaiwulue/kubevpn/cmd/kubevpn/cmds"
)

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Kill, os.Interrupt, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGSTOP, syscall.SIGUSR1)
	go func() {
		<-signals
		cancelFunc()
		<-signals
		os.Exit(1) // second signal. Exit directly.
	}()
	_ = cmds.RootCmd.ExecuteContext(ctx)
}
