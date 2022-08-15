package config

import (
	"net"
	"os"
	"path/filepath"
	"time"

	"k8s.io/client-go/util/homedir"
)

const (
	PodTrafficManager = "kubevpn-traffic-manager"
	UsedIP            = "UsedIP"
	DHCP              = "DHCP"
	Envoy             = "ENVOY_CONFIG"

	SidecarEnvoyProxy   = "envoy-proxy"
	SidecarControlPlane = "control-plane"
	SidecarVPN          = "vpn"

	VolumeEnvoyConfig = "envoy-config"

	s = "223.254.254.100/24"

	Port = 10800

	OriginData string = "origin_data"
	REVERSE    string = "REVERSE"
	Connect    string = "Connect"
	MacToIP    string = "MAC_TO_IP"
	Splitter   string = "#"
)

var (
	// Version inject --ldflags -X
	Version = "latest"

	ImageServer       = "naison/kubevpn:" + Version
	ImageMesh         = "naison/kubevpn-mesh:" + Version
	ImageControlPlane = "naison/envoy-xds-server:" + Version
)

var CIDR *net.IPNet

var RouterIP net.IP

func init() {
	RouterIP, CIDR, _ = net.ParseCIDR(s)
}

var Debug bool = true

var (
	SmallBufferSize  = 2 * 1024  // 2KB small buffer
	MediumBufferSize = 8 * 1024  // 8KB medium buffer
	LargeBufferSize  = 32 * 1024 // 32KB large buffer
)

var (
	KeepAliveTime    = 180 * time.Second
	DialTimeout      = 15 * time.Second
	HandshakeTimeout = 5 * time.Second
	ConnectTimeout   = 5 * time.Second
	ReadTimeout      = 10 * time.Second
	WriteTimeout     = 10 * time.Second
)

var (
	//	network layer ip needs 20 bytes
	//	transport layer UDP header needs 8 bytes
	//	UDP over TCP header needs 22 bytes
	DefaultMTU = 1500 - 20 - 8 - 21
)

var (
	DaemonSock = filepath.Join(homedir.HomeDir(), ".kubevpn", "daemon.sock")
)

func init() {
	if _, err := os.Stat(filepath.Dir(DaemonSock)); os.IsNotExist(err) {
		err = os.Mkdir(filepath.Dir(DaemonSock), 0644)
		if err != nil {
			panic(err)
		}
	}
}
