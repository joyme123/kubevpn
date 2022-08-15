package handler

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	v12 "k8s.io/client-go/kubernetes/typed/core/v1"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

type DHCPManager struct {
	client    v12.ConfigMapInterface
	namespace string
	cidr      *net.IPNet
}

func NewDHCPManager(client v12.ConfigMapInterface, namespace string, addr *net.IPNet) *DHCPManager {
	return &DHCPManager{
		client:    client,
		namespace: namespace,
		cidr:      addr,
	}
}

//	todo optimize dhcp, using mac address, ip and deadline as unit
func (d *DHCPManager) InitDHCPIfNecessary(ctx context.Context) (*v1.ConfigMap, error) {
	cm, err := d.client.Get(ctx, config.PodTrafficManager, metav1.GetOptions{})
	// already exists, do nothing
	if err == nil {
		return cm, nil
	}

	result := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      config.PodTrafficManager,
			Namespace: d.namespace,
			Labels:    map[string]string{},
		},
		Data: map[string]string{
			config.UsedIP: ToString(map[string][]net.IP{util.GetMacAddress().String(): {d.cidr.IP}}),
			config.Envoy:  "",
		},
	}
	cm, err = d.client.Create(ctx, result, metav1.CreateOptions{})
	if err != nil && !errors.IsAlreadyExists(err) {
		log.Errorf("create UsedIP error, err: %v", err)
		return nil, err
	}
	return cm, nil
}

// ToString mac address --> rent ips
func ToString(m map[string][]net.IP) string {
	sb := strings.Builder{}
	for mac, ips := range m {
		strSet := sets.NewString()
		for _, i := range ips {
			strSet.Insert(i.String())
		}
		sb.WriteString(fmt.Sprintf("%s%s%s\\n", mac, config.Splitter, strings.Join(strSet.List(), ",")))
	}
	return sb.String()
}

func FromStringToDHCP(str string) map[string][]net.IP {
	var result = make(map[string][]net.IP)
	for _, line := range strings.Split(str, "\n") {
		if split := strings.Split(line, config.Splitter); len(split) == 2 {
			var ints []net.IP
			for _, s := range strings.Split(split[1], ",") {
				ip := net.ParseIP(s)
				if ip != nil {
					ints = append(ints, ip)
				}
			}
			result[split[0]] = ints
		}
	}
	return result
}

func (d *DHCPManager) RentIP() (*net.IPNet, error) {
	configMap, err := d.client.Get(context.Background(), config.PodTrafficManager, metav1.GetOptions{})
	if err != nil {
		log.Errorf("failed to get ip from dhcp, err: %v", err)
		return nil, err
	}
	//split := strings.Split(get.Data["UsedIP"], ",")
	used := FromStringToDHCP(configMap.Data[config.UsedIP])

	ipRandom, err := d.rentIPRandom()
	if err != nil {
		return nil, err
	}

	v, found := used[util.GetMacAddress().String()]
	if found {
		v = append(v, ipRandom.IP)
	} else {
		v = []net.IP{ipRandom.IP}
	}
	used[util.GetMacAddress().String()] = v

	_, err = d.client.Patch(
		context.Background(),
		configMap.Name,
		types.MergePatchType,
		[]byte(fmt.Sprintf("{\"data\":{\"%s\":\"%s\"}}", config.UsedIP, ToString(used))),
		metav1.PatchOptions{},
	)
	if err != nil {
		log.Errorf("update dhcp error after get ip, need to put ip back, err: %v", err)
		return nil, err
	}

	return ipRandom, nil
}

func (d *DHCPManager) ReleaseIP(ips ...net.IP) error {
	configMap, err := d.client.Get(context.Background(), config.PodTrafficManager, metav1.GetOptions{})
	if err != nil {
		return err
	}
	used := FromStringToDHCP(configMap.Data[config.UsedIP])
	for _, ip := range ips {
		for k := range used {
			v := used[k]
			for i, usedIP := range v {
				if usedIP.Equal(ip) {
					v = append(v[:i], v[i+1:]...)
				}
			}
			used[k] = v
		}
	}
	configMap.Data[config.UsedIP] = ToString(used)
	_, err = d.client.Update(context.Background(), configMap, metav1.UpdateOptions{})
	if err != nil {
		return err
	}
	err = d.ReleaseIpToDHCP(ips...)
	return err
}

type DHCPRecordMap struct {
	innerMap map[string]DHCPRecord
}

//func (maps DHCPRecordMap) MacToIP() map[string]string {
//	result := make(map[string]string)
//	for _, record := range maps.innerMap {
//		result[record.Mac] = record.IP
//	}
//	return result
//}

type DHCPRecord struct {
	Mac      string
	IP       string
	Deadline time.Time
}

// FromStringToMac2IP Mac --> DHCPRecord
func FromStringToMac2IP(str string) (result DHCPRecordMap) {
	result.innerMap = map[string]DHCPRecord{}
	for _, s := range strings.Split(str, "\n") {
		// mac:ip:deadline
		split := strings.Split(s, "#")
		if len(split) == 3 {
			parse, err := time.Parse(time.RFC3339, split[2])
			if err != nil {
				// default deadline is 30min
				parse = time.Now().Add(time.Minute * 30)
			}
			result.innerMap[split[0]] = DHCPRecord{Mac: split[0], IP: split[1], Deadline: parse}
		}
	}
	return
}

func (maps *DHCPRecordMap) ToString() string {
	var sb strings.Builder
	for _, v := range maps.innerMap {
		sb.WriteString(strings.Join([]string{v.Mac, v.IP, v.Deadline.Format(time.RFC3339)}, config.Splitter) + "\\n")
	}
	return sb.String()
}

func (maps *DHCPRecordMap) ToMac2IPMap() map[string]string {
	var result = make(map[string]string)
	for _, record := range maps.innerMap {
		result[record.Mac] = record.IP
	}
	return result
}

func (maps *DHCPRecordMap) GetIPByMac(mac string) (ip string) {
	if record, ok := maps.innerMap[mac]; ok {
		return record.IP
	}
	return
}

func (maps *DHCPRecordMap) AddMacToIPRecord(mac string, ip net.IP) *DHCPRecordMap {
	maps.innerMap[mac] = DHCPRecord{
		Mac:      mac,
		IP:       ip.String(),
		Deadline: time.Now().Add(time.Second * 30),
	}
	return maps
}

func (d *DHCPManager) Release() error {
	configMap, err := d.client.Get(context.Background(), config.PodTrafficManager, metav1.GetOptions{})
	if err != nil {
		return err
	}
	//split := strings.Split(get.Data["UsedIP"], ",")
	used := FromStringToDHCP(configMap.Data[config.UsedIP])
	if rentIPs, found := used[util.GetMacAddress().String()]; found {
		if err = d.ReleaseIP(rentIPs...); err != nil {
			return err
		}
	}
	delete(used, util.GetMacAddress().String())
	configMap.Data[config.UsedIP] = ToString(used)
	_, err = d.client.Update(context.Background(), configMap, metav1.UpdateOptions{})
	return err
}

func (d *DHCPManager) GenerateTunIP(ctx context.Context) (*net.IPNet, error) {
	get, err := d.client.Get(ctx, config.PodTrafficManager, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	mac2IP := FromStringToMac2IP(get.Data[config.MacToIP])
	if ip := mac2IP.GetIPByMac(util.GetMacAddress().String()); len(ip) != 0 {
		return &net.IPNet{IP: net.ParseIP(ip), Mask: d.cidr.Mask}, nil
	}
	localTunIP, err := d.RentIP()
	if err != nil {
		return nil, err
	}
	get, err = d.client.Get(context.TODO(), config.PodTrafficManager, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	data := mac2IP.AddMacToIPRecord(util.GetMacAddress().String(), localTunIP.IP).ToString()
	_, err = d.client.Patch(
		context.TODO(),
		get.Name,
		types.MergePatchType,
		[]byte(fmt.Sprintf("{\"data\":{\"%s\":\"%s\"}}", config.MacToIP, data)),
		metav1.PatchOptions{},
	)
	if err != nil {
		return nil, err
	}
	return localTunIP, nil
}
