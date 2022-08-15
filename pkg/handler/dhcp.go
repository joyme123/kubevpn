package handler

import (
	"context"
	"github.com/cilium/ipam/service/allocator"
	"github.com/cilium/ipam/service/ipallocator"
	log "github.com/sirupsen/logrus"
	"github.com/wencaiwulue/kubevpn/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net"
)

func (d *DHCPManager) rentIPRandom() (*net.IPNet, error) {
	var ipC = make(chan net.IP, 1)
	err := d.updateDHCPConfigMap(func(dhcp *ipallocator.Range) error {
		ip, err := dhcp.AllocateNext()
		if err != nil {
			return err
		}
		ipC <- ip
		return nil
	})
	if err != nil {
		log.Errorf("update dhcp error after get ip, need to put ip back, err: %v", err)
		return nil, err
	}
	return &net.IPNet{IP: <-ipC, Mask: d.cidr.Mask}, nil
}

func (d *DHCPManager) ReleaseIpToDHCP(ips ...net.IP) error {
	return d.updateDHCPConfigMap(func(r *ipallocator.Range) error {
		for _, ip := range ips {
			if err := r.Release(ip); err != nil {
				return err
			}
		}
		return nil
	})
}

func (d *DHCPManager) updateDHCPConfigMap(f func(*ipallocator.Range) error) error {
	cm, err := d.client.Get(context.Background(), config.PodTrafficManager, metav1.GetOptions{})
	if err != nil {
		log.Errorf("failed to get dhcp, err: %v", err)
		return err
	}
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	dhcp, err := ipallocator.NewAllocatorCIDRRange(d.cidr, func(max int, rangeSpec string) (allocator.Interface, error) {
		return allocator.NewContiguousAllocationMap(max, rangeSpec), nil
	})
	if err != nil {
		return err
	}
	if err = dhcp.Restore(d.cidr, []byte(cm.Data[config.DHCP])); err != nil {
		return err
	}
	if err = f(dhcp); err != nil {
		return err
	}
	_, bytes, err := dhcp.Snapshot()
	if err != nil {
		return err
	}
	cm.Data[config.DHCP] = string(bytes)
	_, err = d.client.Update(context.Background(), cm, metav1.UpdateOptions{})
	if err != nil {
		log.Errorf("update dhcp failed, err: %v", err)
		return err
	}
	return nil
}
