package config

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
	"k8s.io/client-go/util/homedir"
)

func TestName(t *testing.T) {
	listen, _ := net.Listen("tcp", ":9090")
	listener := tls.NewListener(listen, TlsConfigServer)
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				log.Errorln(err)
			}
			go func(conn net.Conn) {
				bytes := make([]byte, 1024)
				all, err2 := conn.Read(bytes)
				if err2 != nil {
					log.Errorln(err2)
					return
				}
				defer conn.Close()
				fmt.Println(string(bytes[:all]))
				io.WriteString(conn, "hello client")
			}(conn)
		}
	}()
	dial, err := net.Dial("tcp", ":9090")
	if err != nil {
		log.Errorln(err)
	}

	client := tls.Client(dial, TlsConfigClient)
	client.Write([]byte("hi server"))
	all, err := io.ReadAll(client)
	if err != nil {
		log.Errorln(err)
	}
	fmt.Println(string(all))
}

func TestCreateFile(t *testing.T) {
	var listen net.Listener
	var err error
	go func() {
		listen, err = net.Listen("unix", filepath.Join(homedir.HomeDir(), ".kubevpn", "test111"))
		if err != nil {
			t.Error(err)
			t.Fatal(err)
		}
	}()
	time.Sleep(time.Second * 2)
	defer listen.Close()
	var createFileFuncSocketsss = func(path string) {
		if _, err := os.Stat(filepath.Dir(path)); os.IsNotExist(err) {
			_ = os.Mkdir(filepath.Dir(path), 0644)
		}
		_ = os.Chmod(path, os.ModeSocket|os.ModePerm)

	}
	createFileFuncSocketsss(filepath.Join(homedir.HomeDir(), ".kubevpn", "test111"))
	time.Sleep(time.Second * 100)
}

func TestLog(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	//log.SetReportCaller(false)

	go func() {
		pipe, writer := io.Pipe()
		go func() {
			reader := bufio.NewReader(pipe)
			for {
				line, _, err := reader.ReadLine()
				if err != nil {
					return
				}
				println(string(line))
			}
		}()
		log.SetOutput(io.MultiWriter(os.Stderr, writer))
		log.Debugf("world")
	}()
	log.Debugf("hello")
	time.Sleep(time.Second * 1)
}
