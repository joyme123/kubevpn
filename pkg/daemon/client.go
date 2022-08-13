package daemon

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/wencaiwulue/kubevpn/pkg/config"
	"io"
	"net"
	"os"
	"strconv"
	"syscall"
	"time"
)

func GetClient(strings ...string) *client {
	var p = config.DaemonSock
	if len(strings) != 0 {
		p = strings[0]
	}
	return &client{socketPath: p}
}

type client struct {
	socketPath string
}

func (c client) SendJsonRequest(ctx context.Context, req interface{}, resp interface{}) error {
	_, err := os.Lstat(c.socketPath)
	if err != nil {
		return err
	}

	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", c.socketPath, time.Second*5)
	if err != nil {
		return err
	}
	bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(bytes, uint16(len(reqBytes)))
	n, err := conn.Write(bytes)
	if err != nil {
		return err
	}
	if n != 2 {
		return fmt.Errorf("length is not equal to 2")
	}

	write, err := conn.Write(reqBytes)
	if err != nil {
		return err
	}
	if write != len(reqBytes) {
		return fmt.Errorf("write length is not equal to req bytes")
	}

	go func(cancelCtx context.Context) {
		<-cancelCtx.Done()
		_, _ = conn.Write([]byte(strconv.Itoa(int(syscall.SIGKILL)) + "\n"))
	}(cancelCtx)

	all, err := io.ReadAll(conn)
	if err != nil {
		return err
	}
	if len(all) == 0 {
		return nil
	}
	if resp == nil {
		return nil
	}
	err = json.Unmarshal(all, &resp)
	if err != nil {
		return errors.New(string(all))
	}
	return nil
}

func (c client) SendStreamRequest(ctx context.Context, req interface{}, consume func(reader io.Reader) error) error {
	cancelCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	marshal, err := json.Marshal(req)
	if err != nil {
		return err
	}
	conn, err := net.DialTimeout("unix", c.socketPath, time.Second*5)
	if err != nil {
		return err
	}

	bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(bytes, uint16(len(marshal)))
	n, err := conn.Write(bytes)
	if err != nil {
		return err
	}
	if n != 2 {
		return fmt.Errorf("")
	}

	write, err := conn.Write(marshal)
	if err != nil {
		return err
	}
	if write != len(marshal) {
		return fmt.Errorf("")
	}
	go func(cancelCtx context.Context) {
		<-cancelCtx.Done()
		_, _ = conn.Write([]byte(strconv.Itoa(int(syscall.SIGKILL)) + "\n"))
	}(cancelCtx)

	return consume(conn)
}
