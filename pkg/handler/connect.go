package handler

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/util/retry"
	"net"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	"github.com/wencaiwulue/kubevpn/pkg/core"
	"github.com/wencaiwulue/kubevpn/pkg/dns"
	"github.com/wencaiwulue/kubevpn/pkg/route"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

type ConnectOptions struct {
	NamespaceID    types.UID
	KubeconfigPath string
	Namespace      string
	clientset      *kubernetes.Clientset
	restclient     *rest.RESTClient
	config         *rest.Config
	factory        cmdutil.Factory
	cidrs          []*net.IPNet
	dhcp           *DHCPManager
	route          Route
	localTunIP     *net.IPNet
}

func (c *ConnectOptions) InitDHCP(ctx context.Context) error {
	err := c.dhcp.InitDHCPIfNecessary(ctx)
	if err != nil {
		return err
	}
	c.localTunIP, err = c.dhcp.GenerateTunIP(ctx, true)
	if err != nil {
		return err
	}
	return nil
}

func (c *ConnectOptions) DoConnect(ctx context.Context, connectCtx context.Context, logger *log.Logger) (err error) {
	// 1) get CIDR range from cluster
	c.cidrs, err = getCIDR(ctx, c.clientset, c.Namespace)
	if err != nil {
		return
	}
	var sb []string
	for _, cidr := range c.cidrs {
		sb = append(sb, cidr.String())
	}
	logger.Infoln("CIDR are: " + strings.Join(sb, ","))

	// 2) init UsedIP server
	trafficMangerNet := net.IPNet{IP: config.RouterIP, Mask: config.CIDR.Mask}
	c.dhcp = NewDHCPManager(c.clientset.CoreV1().ConfigMaps(c.Namespace), c.Namespace, &trafficMangerNet)
	err = c.InitDHCP(ctx)
	if err != nil {
		return
	}

	// 3) create pods traffic-manager
	err = CreateOutboundPod(c.clientset, c.Namespace, trafficMangerNet.String(), c.cidrs, logger)
	if err != nil {
		return
	}

	// 4) port-forward port 10800 for traffic-manager
	subCtx, cancelFunc := context.WithTimeout(ctx, time.Minute*2)
	defer cancelFunc()
	logger.Infoln(fmt.Sprintf("wait port %v to be free...", config.Port))
	err = util.WaitPortToBeFree(subCtx, config.Port)
	if err != nil {
		return err
	}
	logger.Infoln(fmt.Sprintf("port %v are free", config.Port))
	if err = c.portForward(connectCtx, config.Port); err != nil {
		return err
	}
	logger.Infoln("port forward ready")

	// 5) start local tunnel service
	c.setRouteInfo()
	logger.Info("your ip is " + c.localTunIP.IP.String())
	util.InitLogger(true)
	err = StartTunServer(connectCtx, c.route)
	if err != nil {
		logger.Errorf("error while create tunnel, err: %v", err)
		return err
	}
	logger.Info("tunnel connected")

	// 6) delete firewall rule for Windows os
	c.deleteFirewallRule(ctx)

	// 7) setup dns
	err = c.setupDNS(ctx)
	if err != nil {
		return err
	}
	logger.Info("dns service ok")

	// 8) detect and disable conflict device
	err = c.detectConflictDevice()

	if err != nil {
		return err
	}
	return
}

// detect pod is delete event, if pod is deleted, needs to redo port-forward immediately
func (c *ConnectOptions) portForward(ctx context.Context, port int) error {
	var childCtx context.Context
	var cancelFunc context.CancelFunc
	var curPodName = &atomic.Value{}
	var readyChan = make(chan struct{}, 1)
	var errChan = make(chan error, 1)
	var first = true
	podInterface := c.clientset.CoreV1().Pods(c.Namespace)
	go func() {
		for ctx.Err() == nil {
			func() {
				err := retry.OnError(wait.Backoff{
					Steps:    10,
					Duration: 1 * time.Second,
					Factor:   2.0,
				}, func(err error) bool {
					return err != nil
				}, func() error {
					podList, err := c.GetRunningPodList(ctx)
					if err == nil {
						curPodName.Store(podList[0].Name)
					}
					return err
				})
				if err != nil {
					return
				}
				childCtx, cancelFunc = context.WithCancel(ctx)
				defer cancelFunc()
				if !first {
					readyChan = nil
				}
				podName := curPodName.Load().(string)
				// if port-forward occurs error, check pod is deleted or not, speed up fail
				runtime.ErrorHandlers = []func(error){func(err error) {
					pod, err := podInterface.Get(childCtx, podName, metav1.GetOptions{})
					if apierrors.IsNotFound(err) || pod.GetDeletionTimestamp() != nil {
						cancelFunc()
					}
				}}

				// try to detect pod is delete event, if pod is deleted, needs to redo port-forward
				go func(podName string) {
					for childCtx.Err() == nil {
						func() {
							stream, err := podInterface.Watch(childCtx, metav1.ListOptions{
								FieldSelector: fields.OneTermEqualSelector("metadata.name", podName).String(),
							})
							if apierrors.IsNotFound(err) {
								cancelFunc()
								return
							}
							if err != nil {
								time.Sleep(30 * time.Second)
								return
							}
							defer stream.Stop()
							for childCtx.Err() == nil {
								select {
								case e, ok := <-stream.ResultChan():
									if ok && e.Type == watch.Deleted {
										cancelFunc()
										return
									}
								}
							}
						}()
					}
				}(podName)

				err = util.PortForwardPod(
					c.config,
					c.restclient,
					podName,
					c.Namespace,
					strconv.Itoa(port),
					readyChan,
					childCtx.Done(),
				)
				if first {
					errChan <- err
				}
				first = false
				// exit normal, let context.err to judge to exit or not
				if err == nil {
					return
				}
				if strings.Contains(err.Error(), "unable to listen on any of the requested ports") ||
					strings.Contains(err.Error(), "address already in use") {
					log.Errorf("port %d already in use, needs to release it manually", port)
					time.Sleep(time.Second * 5)
				} else {
					log.Errorf("port-forward occurs error, err: %v, retrying", err)
					time.Sleep(time.Second * 2)
				}
			}()
		}
	}()

	select {
	case <-time.Tick(time.Second * 60):
		return errors.New("port forward timeout")
	case err := <-errChan:
		return err
	case <-readyChan:
		return nil
	}
}

func (c *ConnectOptions) setRouteInfo() {
	// todo figure it out why
	if util.IsWindows() {
		c.localTunIP.Mask = net.CIDRMask(0, 32)
	}
	var list = []string{config.CIDR.String()}
	for _, ipNet := range c.cidrs {
		list = append(list, ipNet.String())
	}
	c.route = Route{
		ServeNodes: []string{
			fmt.Sprintf("tun:/127.0.0.1:8422?net=%s&route=%s", c.localTunIP.String(), strings.Join(list, ",")),
		},
		ChainNode: fmt.Sprintf("tcp://%s:%d", "127.0.0.1", config.Port),
		Retries:   5,
	}
}

func (c *ConnectOptions) deleteFirewallRule(ctx context.Context) {
	if util.IsWindows() {
		if !util.FindRule() {
			util.AddFirewallRule()
		}
		go util.DeleteWindowsFirewallRule(ctx)
	}
	go util.Heartbeats(ctx)
}

func (c *ConnectOptions) detectConflictDevice() error {
	tun := os.Getenv("tunName")
	if len(tun) == 0 {
		return errors.New("can not found tun device")
	}
	err := route.DetectAndDisableConflictDevice(tun)
	if err != nil {
		return fmt.Errorf("error occours while disable conflict devices, err: %v", err)
	}
	return nil
}

func (c *ConnectOptions) setupDNS(ctx context.Context) error {
	podList, err := c.GetRunningPodList(ctx)
	if err != nil {
		return err
	}
	relovConf, err := dns.GetDNSServiceIPFromPod(c.clientset, c.restclient, c.config, podList[0].GetName(), c.Namespace)
	if err != nil {
		return err
	}
	if err = dns.SetupDNS(relovConf); err != nil {
		return err
	}
	return nil
}

func StartTunServer(ctx context.Context, r Route) error {
	servers, err := r.GenerateServers()
	if err != nil {
		return errors.WithStack(err)
	}
	if len(servers) == 0 {
		return errors.New("invalid route config")
	}
	for _, rr := range servers {
		go func(ctx context.Context, server core.Server) {
			if err = server.Serve(ctx); err != nil {
				log.Debug(err)
			}
		}(ctx, rr)
	}
	return nil
}

func getCIDR(ctx2 context.Context, clientset *kubernetes.Clientset, namespace string) ([]*net.IPNet, error) {
	var CIDRList []*net.IPNet
	// get pod CIDR from node spec
	if nodeList, err := clientset.CoreV1().Nodes().List(ctx2, metav1.ListOptions{}); err == nil {
		var podCIDRs = sets.NewString()
		for _, node := range nodeList.Items {
			if node.Spec.PodCIDRs != nil {
				podCIDRs.Insert(node.Spec.PodCIDRs...)
			}
			if len(node.Spec.PodCIDR) != 0 {
				podCIDRs.Insert(node.Spec.PodCIDR)
			}
		}
		for _, podCIDR := range podCIDRs.List() {
			if _, CIDR, err := net.ParseCIDR(podCIDR); err == nil {
				CIDRList = append(CIDRList, CIDR)
			}
		}
	}
	// get pod CIDR from pod ip, why doing this: notice that node's pod cidr is not correct in minikube
	// ➜  ~ kubectl get nodes -o jsonpath='{.items[*].spec.podCIDR}'
	//10.244.0.0/24%
	// ➜  ~  kubectl get pods -o=custom-columns=podIP:.status.podIP
	//podIP
	//172.17.0.5
	//172.17.0.4
	//172.17.0.4
	//172.17.0.3
	//172.17.0.3
	//172.17.0.6
	//172.17.0.8
	//172.17.0.3
	//172.17.0.7
	//172.17.0.2
	if podList, err := clientset.CoreV1().Pods(namespace).List(ctx2, metav1.ListOptions{}); err == nil {
		for _, pod := range podList.Items {
			if ip := net.ParseIP(pod.Status.PodIP); ip != nil {
				var contain bool
				for _, CIDR := range CIDRList {
					if CIDR.Contains(ip) {
						contain = true
						break
					}
				}
				if !contain {
					mask := net.CIDRMask(24, 32)
					CIDRList = append(CIDRList, &net.IPNet{IP: ip.Mask(mask), Mask: mask})
				}
			}
		}
	}

	// get service CIDR
	defaultCIDRIndex := "The range of valid IPs is"
	if _, err := clientset.CoreV1().Services(namespace).Create(ctx2, &v1.Service{
		ObjectMeta: metav1.ObjectMeta{GenerateName: "foo-svc-"},
		Spec:       v1.ServiceSpec{Ports: []v1.ServicePort{{Port: 80}}, ClusterIP: "0.0.0.0"},
	}, metav1.CreateOptions{}); err != nil {
		idx := strings.LastIndex(err.Error(), defaultCIDRIndex)
		if idx != -1 {
			if _, cidr, err := net.ParseCIDR(strings.TrimSpace(err.Error()[idx+len(defaultCIDRIndex):])); err == nil {
				CIDRList = append(CIDRList, cidr)
			}
		}
	} else {
		if serviceList, err := clientset.CoreV1().Services(namespace).List(ctx2, metav1.ListOptions{}); err == nil {
			for _, service := range serviceList.Items {
				if ip := net.ParseIP(service.Spec.ClusterIP); ip != nil {
					var contain bool
					for _, CIDR := range CIDRList {
						if CIDR.Contains(ip) {
							contain = true
							break
						}
					}
					if !contain {
						mask := net.CIDRMask(16, 32)
						CIDRList = append(CIDRList, &net.IPNet{IP: ip.Mask(mask), Mask: mask})
					}
				}
			}
		}
	}

	// remove duplicate CIDR
	result := make([]*net.IPNet, 0)
	set := sets.NewString()
	for _, cidr := range CIDRList {
		if !set.Has(cidr.String()) {
			set.Insert(cidr.String())
			result = append(result, cidr)
		}
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("can not found any CIDR")
	}
	return result, nil
}

func (c *ConnectOptions) InitClient() (err error) {
	configFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	if _, err = os.Stat(c.KubeconfigPath); err == nil {
		configFlags.KubeConfig = &c.KubeconfigPath
	}

	c.factory = cmdutil.NewFactory(cmdutil.NewMatchVersionFlags(configFlags))

	if c.config, err = c.factory.ToRESTConfig(); err != nil {
		return
	}
	if c.restclient, err = c.factory.RESTClient(); err != nil {
		return
	}
	if c.clientset, err = c.factory.KubernetesClientSet(); err != nil {
		return
	}
	if len(c.Namespace) == 0 {
		if c.Namespace, _, err = c.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
			return
		}
	}
	c.NamespaceID, err = util.GetNamespaceId(c.clientset.CoreV1().Namespaces(), c.Namespace)
	if err != nil {
		return err
	}
	return
}

func (c *ConnectOptions) GetRunningPodList(ctx context.Context) ([]v1.Pod, error) {
	list, err := c.clientset.CoreV1().Pods(c.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fields.OneTermEqualSelector("app", config.PodTrafficManager).String(),
	})
	if err != nil {
		return nil, err
	}
	for i := 0; i < len(list.Items); i++ {
		if list.Items[i].GetDeletionTimestamp() != nil || list.Items[i].Status.Phase != v1.PodRunning {
			list.Items = append(list.Items[:i], list.Items[i+1:]...)
			i--
		}
	}
	if len(list.Items) == 0 {
		return nil, errors.New("can not found any running pod")
	}
	return list.Items, nil
}

func (c *ConnectOptions) Cleanup() {
	log.Infoln("prepare to exit, cleaning up")
	dns.CancelDNS()

	err := c.dhcp.Release()
	if err != nil {
		log.Errorln(err)
	}

	cleanUpTrafficManagerIfRefCountIsZero(c.clientset, c.Namespace)
	log.Infoln("clean up successful")
}
