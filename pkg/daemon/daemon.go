package daemon

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"reflect"
	"strconv"
	"syscall"
	"time"

	log "github.com/sirupsen/logrus"
)

var Uptime int64
var ctx context.Context
var cancel context.CancelFunc

type Options struct {
	SockPath string
}

func (o *Options) Start(pCtx context.Context) error {
	listen, err := net.Listen("unix", o.SockPath)
	if err != nil {
		return err
	}
	_ = os.Chmod(o.SockPath, os.ModeSocket|os.ModePerm)
	ctx, cancel = context.WithCancel(pCtx)
	Uptime = time.Now().Unix()
	go func() {
		for ctx.Err() == nil {
			var conn net.Conn
			conn, err = listen.Accept()
			if err != nil {
				log.Errorln(err)
				continue
			}
			go handle(conn)
		}
	}()
	<-ctx.Done()
	_ = listen.Close()
	return nil
}

func (o *Options) Stop() {
	cancel()
}

func handle(conn net.Conn) {
	var err error
	defer conn.Close()
	defer func() {
		if err != nil {
			_, _ = conn.Write([]byte(err.Error()))
		}
	}()
	err = conn.SetReadDeadline(time.Now().Add(time.Second * 10))
	if err != nil {
		return
	}
	var b = make([]byte, 2)
	var n int
	// first [2]byte are data length
	n, err = io.ReadFull(conn, b)
	if err != nil {
		return
	}
	if n != 2 {
		return
	}
	dataLength := binary.BigEndian.Uint16(b)

	bytes := make([]byte, dataLength)
	var read int
	read, err = io.ReadFull(conn, bytes)
	if err != nil {
		return
	}
	if read != int(dataLength) {
		return
	}

	var comm CommonAction
	err = json.Unmarshal(bytes, &comm)
	if err != nil {
		return
	}
	cancelCtx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	// listen for command ctrl+c event
	go func(cancelFunc context.CancelFunc) {
		for {
			line, _, err := bufio.NewReader(conn).ReadLine()
			if err != nil {
				return
			}

			var i int
			i, err = strconv.Atoi(string(line))
			if err == nil {
				signal := syscall.Signal(i)
				switch signal {
				case syscall.SIGKILL, syscall.SIGQUIT, syscall.SIGSTOP:
					cancelFunc()
					return
				}
			}
		}
	}(cancelFunc)

	action, ok := HandlerMap[comm.Action]
	if !ok {
		err = fmt.Errorf("not support action: %s", comm.Action)
		return
	}

	c := reflect.New(reflect.TypeOf(action).Elem()).Interface()
	err = json.Unmarshal(bytes, c)
	if err != nil {
		return
	}
	switch c.(type) {
	case JsonHandler:
		var resp interface{}
		resp, err = c.(JsonHandler).HandleJson(cancelCtx)
		if err != nil {
			return
		}
		if resp != nil {
			var result []byte
			result, err = json.Marshal(resp)
			if err != nil {
				return
			}
			_, err = conn.Write(result)
		}
		return
	case StreamHandler:
		err = c.(StreamHandler).HandleStream(cancelCtx, conn)
		return
	default:
		err = fmt.Errorf("not support function: %v", c)
		return
	}

}
