//go:build linux
// +build linux

package dns

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/docker/docker/libnetwork/resolvconf"
	miekgdns "github.com/miekg/dns"
	log "github.com/sirupsen/logrus"

	"github.com/wencaiwulue/kubevpn/pkg/config"
)

// systemd-resolve --status, systemd-resolve --flush-caches
func SetupDNS(clientConfig *miekgdns.ClientConfig, _ []string) error {
	tunName := os.Getenv(config.EnvTunNameOrLUID)
	if len(tunName) == 0 {
		tunName = "tun0"
	}
	// TODO consider use https://wiki.debian.org/NetworkManager and nmcli to config DNS
	// try to solve:
	// sudo systemd-resolve --set-dns 172.28.64.10 --interface tun0 --set-domain=vke-system.svc.cluster.local --set-domain=svc.cluster.local --set-domain=cluster.local
	//Failed to set DNS configuration: Unit dbus-org.freedesktop.resolve1.service not found.
	// ref: https://superuser.com/questions/1427311/activation-via-systemd-failed-for-unit-dbus-org-freedesktop-resolve1-service
	// systemctl enable systemd-resolved.service
	_ = exec.Command("systemctl", "enable", "systemd-resolved.service").Run()
	// systemctl start systemd-resolved.service
	_ = exec.Command("systemctl", "start", "systemd-resolved.service").Run()
	//systemctl status systemd-resolved.service
	_ = exec.Command("systemctl", "status", "systemd-resolved.service").Run()

	cmd := exec.Command("systemd-resolve", []string{
		"--set-dns",
		clientConfig.Servers[0],
		"--interface",
		tunName,
		"--set-domain=" + clientConfig.Search[0],
		"--set-domain=" + clientConfig.Search[1],
		"--set-domain=" + clientConfig.Search[2],
	}...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Debugf("failed to exec cmd: %s, message: %s, ignore", strings.Join(cmd.Args, " "), string(output))
	}

	filename := filepath.Join("/", "etc", "resolv.conf")
	readFile, err := os.ReadFile(filename)
	if err == nil {
		resolvConf, err := miekgdns.ClientConfigFromReader(bytes.NewBufferString(string(readFile)))
		if err == nil {
			if len(resolvConf.Servers) != 0 {
				clientConfig.Servers = append(clientConfig.Servers, resolvConf.Servers...)
			}
			if len(resolvConf.Search) != 0 {
				clientConfig.Search = append(clientConfig.Search, resolvConf.Search...)
			}
		}
	}

	return WriteResolvConf(*clientConfig)
}

func CancelDNS() {
	updateHosts("")

	filename := filepath.Join("/", "etc", "resolv.conf")
	_ = os.Rename(getBackupFilename(filename), filename)
}

func GetHostFile() string {
	return "/etc/hosts"
}

func WriteResolvConf(config miekgdns.ClientConfig) error {
	var options []string
	if config.Ndots != 0 {
		options = append(options, fmt.Sprintf("ndots:%d", config.Ndots))
	}
	if config.Attempts != 0 {
		options = append(options, fmt.Sprintf("attempts:%d", config.Attempts))
	}
	if config.Timeout != 0 {
		options = append(options, fmt.Sprintf("timeout:%d", config.Timeout))
	}

	filename := filepath.Join("/", "etc", "resolv.conf")
	_ = os.Rename(filename, getBackupFilename(filename))
	_, err := resolvconf.Build(filename, config.Servers, config.Search, options)
	return err
}

func getBackupFilename(filename string) string {
	return filename + ".kubevpn_backup"
}
