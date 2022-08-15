package handler

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	cmdutil "k8s.io/kubectl/pkg/cmd/util"
	"k8s.io/kubectl/pkg/polymorphichelpers"
	"net"
	"os"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	"github.com/wencaiwulue/kubevpn/pkg/util"
)

type Mode string

const (
	Mesh    Mode = "mesh"
	Reverse Mode = "reverse"
)

type ReverseOptions struct {
	KubeconfigPath string
	Namespace      string
	NamespaceID    types.UID
	Mode           Mode
	Headers        map[string]string
	Workloads      []string
	clientset      *kubernetes.Clientset
	restclient     *rest.RESTClient
	config         *rest.Config
	factory        cmdutil.Factory
	cidrs          []*net.IPNet
	dhcp           *DHCPManager
	// needs to give it back to dhcp
	usedIPs    []*net.IPNet
	routerIP   net.IP
	LocalTunIP *net.IPNet
}

func (r *ReverseOptions) GetDHCP() *DHCPManager {
	return r.dhcp
}

func (r *ReverseOptions) GetClient() *kubernetes.Clientset {
	return r.clientset
}

func (r *ReverseOptions) GetNamespace() string {
	return r.Namespace
}

func (r *ReverseOptions) InitDHCP(ctx context.Context) error {
	_, err := r.dhcp.InitDHCPIfNecessary(ctx)
	if err != nil {
		return err
	}
	r.LocalTunIP, err = r.dhcp.GenerateTunIP(ctx)
	return err
}

func (r *ReverseOptions) createRemoteInboundPod(ctx context.Context) error {
	tempIps := []*net.IPNet{r.LocalTunIP}
	for _, workload := range r.Workloads {
		if len(workload) > 0 {
			virtualShadowIp, err := r.dhcp.RentIP()
			if err != nil {
				return err
			}

			tempIps = append(tempIps, virtualShadowIp)
			configInfo := config.PodRouteConfig{
				LocalTunIP:           r.LocalTunIP.IP.String(),
				InboundPodTunIP:      virtualShadowIp.String(),
				TrafficManagerRealIP: r.routerIP.String(),
				Route:                config.CIDR.String(),
			}
			// TODO OPTIMIZE CODE
			if r.Mode == Mesh {
				err = InjectVPNAndEnvoySidecar(ctx, r.factory, r.clientset.CoreV1().ConfigMaps(r.Namespace), r.Namespace, workload, configInfo, r.Headers)
			} else {
				err = InjectVPNSidecar(ctx, r.factory, r.Namespace, workload, configInfo)
			}
			if err != nil {
				return err
			}
		}
	}
	r.usedIPs = tempIps
	return nil
}

func (r *ReverseOptions) DoReverse(ctx context.Context, logger *log.Logger) (err error) {
	r.cidrs, err = getCIDR(ctx, r.clientset, r.Namespace)
	if err != nil {
		return
	}
	trafficMangerNet := net.IPNet{IP: config.RouterIP, Mask: config.CIDR.Mask}
	r.dhcp = NewDHCPManager(r.clientset.CoreV1().ConfigMaps(r.Namespace), r.Namespace, &trafficMangerNet)
	err = r.InitDHCP(ctx)
	if err != nil {
		return
	}

	var found bool
	r.routerIP, found = getOutBoundService(r.clientset.CoreV1().Services(r.Namespace))
	if !found {
		err = errors.New("can not found outbound service")
		return
	}

	logger.Debugln("try to create remote inbound pod...")
	err = r.createRemoteInboundPod(ctx)
	if err != nil {
		return
	}
	logger.Debugln("try to create remote inbound pod ok")
	return
}

func (r *ReverseOptions) InitClient() (err error) {
	configFlags := genericclioptions.NewConfigFlags(true).WithDeprecatedPasswordFlag()
	if _, err = os.Stat(r.KubeconfigPath); err == nil {
		configFlags.KubeConfig = &r.KubeconfigPath
	}

	r.factory = cmdutil.NewFactory(cmdutil.NewMatchVersionFlags(configFlags))

	if r.config, err = r.factory.ToRESTConfig(); err != nil {
		return
	}
	if r.restclient, err = r.factory.RESTClient(); err != nil {
		return
	}
	if r.clientset, err = r.factory.KubernetesClientSet(); err != nil {
		return
	}
	if len(r.Namespace) == 0 {
		if r.Namespace, _, err = r.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
			return
		}
	}
	return
}

// PreCheckResource transform user parameter to normal, example:
// pod: productpage-7667dfcddb-cbsn5
// replicast: productpage-7667dfcddb
// deployment: productpage
// transform:
// pod/productpage-7667dfcddb-cbsn5 --> deployment/productpage
// service/productpage --> deployment/productpage
// replicaset/productpage-7667dfcddb --> deployment/productpage
//
// pods without controller
// pod/productpage-without-controller --> pod/productpage-without-controller
// service/productpage-without-pod --> controller/controllerName
func (r *ReverseOptions) PreCheckResource() {
	// normal workloads, like pod with controller, deployments, statefulset, replicaset etc...
	for i, workload := range r.Workloads {
		ownerReference, err := util.GetTopOwnerReference(r.factory, r.Namespace, workload)
		if err == nil {
			r.Workloads[i] = fmt.Sprintf("%s/%s", ownerReference.Mapping.GroupVersionKind.GroupKind().String(), ownerReference.Name)
		}
	}
	// service which associate with pod
	for i, workload := range r.Workloads {
		object, err := util.GetUnstructuredObject(r.factory, r.Namespace, workload)
		if err != nil {
			continue
		}
		if object.Mapping.Resource.Resource != "services" {
			continue
		}
		get, err := r.clientset.CoreV1().Services(r.Namespace).Get(context.TODO(), object.Name, metav1.GetOptions{})
		if err != nil {
			continue
		}
		if ns, selector, err := polymorphichelpers.SelectorsForObject(get); err == nil {
			list, err := r.clientset.CoreV1().Pods(ns).List(context.TODO(), metav1.ListOptions{
				LabelSelector: selector.String(),
			})
			// if pod is not empty, using pods to find top controller
			if err == nil && list != nil && len(list.Items) != 0 {
				ownerReference, err := util.GetTopOwnerReference(r.factory, r.Namespace, fmt.Sprintf("%s/%s", "pods", list.Items[0].Name))
				if err == nil {
					r.Workloads[i] = fmt.Sprintf("%s/%s", ownerReference.Mapping.GroupVersionKind.GroupKind().String(), ownerReference.Name)
				}
			} else
			// if list is empty, means not create pods, just controllers
			{
				controller, err := util.GetTopOwnerReferenceBySelector(r.factory, r.Namespace, selector.String())
				if err == nil {
					if len(controller) > 0 {
						r.Workloads[i] = controller.List()[0]
					}
				}
				// only a single service, not support it yet
				if controller.Len() == 0 {
					log.Fatalf("Not support resources: %s", workload)
				}
			}
		}
	}
}

func (r *ReverseOptions) GetRunningPodList(ctx context.Context) ([]v1.Pod, error) {
	list, err := r.clientset.CoreV1().Pods(r.Namespace).List(ctx, metav1.ListOptions{
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

func (r *ReverseOptions) GetNamespaceId() (types.UID, error) {
	if r.NamespaceID != "" {
		return r.NamespaceID, nil
	}
	namespace, err := r.clientset.CoreV1().Namespaces().Get(context.Background(), r.Namespace, metav1.GetOptions{})
	if err != nil {
		return "", err
	}
	r.NamespaceID = namespace.GetUID()
	return r.NamespaceID, nil
}
