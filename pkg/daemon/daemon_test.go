package daemon

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/wencaiwulue/kubevpn/pkg/config"
)

func TestDamonClient(t *testing.T) {
	HandlerMap["sleep"] = &sleep{}
	go func() {
		(&Options{SockPath: config.DaemonSock}).Start(context.Background())
	}()
	time.Sleep(time.Millisecond * 200)
	cancel, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go func() {
		time.AfterFunc(time.Second*2, func() {
			cancelFunc()
		})
	}()
	var m = make(map[string]string)
	err := GetClient().SendJsonRequest(cancel, map[string]string{"action": "sleep", "contentType": "json"}, &m)
	if err != nil {
		t.Error(err)
	}
}

func TestDamonClientStream(t *testing.T) {
	HandlerMap["sleep"] = &sleep{}
	go func() {
		(&Options{SockPath: config.DaemonSock}).Start(context.Background())
	}()
	time.Sleep(time.Millisecond * 200)
	cancel, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go func() {
		time.AfterFunc(time.Second*10, func() {
			cancelFunc()
		})
	}()
	err := GetClient().SendStreamRequest(cancel, map[string]string{"action": "sleep", "contentType": "stream"}, func(reader io.Reader) error {
		_, _ = io.Copy(os.Stdout, reader)
		return nil
	})
	if err != nil {
		t.Error(err)
	}
}

func TestMap(t *testing.T) {
	var m = map[string]string{"m": "mm"}

	for i := 0; i < 10000; i++ {
		go func(i int) {
			if i%2 == 0 {
				_ = m["m"]
			} /* else {
				m["m"] = "mm"
			}*/
		}(i)
	}

	time.Sleep(time.Second)
}

type sleep struct {
	Name string `json:"name"`
}

func (receiver sleep) HandleJson(ctx context.Context) (interface{}, error) {
	cancel, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	go func() {
		time.Sleep(time.Second * 10)
		cancelFunc()
	}()
	<-cancel.Done()
	return "well done", nil
}

func (receiver sleep) HandleStream(ctx context.Context, resp io.Writer) error {
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	go func() {
		resp.Write([]byte(("hi client\n")))
		time.Sleep(time.Second)
		resp.Write([]byte(("i am doing that, please wait...\n")))
		time.Sleep(time.Second)
		resp.Write([]byte(("i am doing that, please wait...\n")))
		time.Sleep(time.Second)
		resp.Write([]byte("i am done\n"))
		cancelFunc()
	}()
	<-cancelCtx.Done()
	return nil
}
