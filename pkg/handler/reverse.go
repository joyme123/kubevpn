package handler

import (
	"context"
	"fmt"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/kubernetes"
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

	factory   cmdutil.Factory
	clientset *kubernetes.Clientset
	dhcp      *DHCPManager

	Mode      Mode
	Headers   map[string]string
	Workloads []string
}

func (r *ReverseOptions) createRemoteInboundPod(ctx context.Context) error {
	routerIP, found := getOutBoundService(r.clientset.CoreV1().Services(r.Namespace))
	if !found {
		return errors.New("can not found outbound service")
	}

	localTunIP, err := r.dhcp.GenerateTunIP(ctx, false)
	if err != nil {
		return err
	}

	for _, workload := range r.Workloads {
		var virtualShadowIp *net.IPNet
		virtualShadowIp, err = r.dhcp.RentIP()
		if err != nil {
			return err
		}

		configInfo := config.PodRouteConfig{
			LocalTunIP:           localTunIP.IP.String(),
			InboundPodTunIP:      virtualShadowIp.String(),
			TrafficManagerRealIP: routerIP.String(),
			Route:                config.CIDR.String(),
		}
		if r.Mode == Mesh {
			err = InjectVPNAndEnvoySidecar(ctx, r.factory, r.clientset.CoreV1().ConfigMaps(r.Namespace), r.Namespace, workload, configInfo, r.Headers)
		} else {
			err = InjectVPNSidecar(ctx, r.factory, r.Namespace, workload, configInfo)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *ReverseOptions) DoReverse(ctx context.Context, logger *log.Logger) (err error) {
	r.dhcp = NewDHCPManager(r.clientset.CoreV1().ConfigMaps(r.Namespace), r.Namespace, &net.IPNet{IP: config.RouterIP, Mask: config.CIDR.Mask})
	err = r.dhcp.InitDHCPIfNecessary(ctx)
	if err != nil {
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

	if r.clientset, err = r.factory.KubernetesClientSet(); err != nil {
		return
	}
	if len(r.Namespace) == 0 {
		if r.Namespace, _, err = r.factory.ToRawKubeConfigLoader().Namespace(); err != nil {
			return
		}
	}
	r.NamespaceID, err = util.GetNamespaceId(r.clientset.CoreV1().Namespaces(), r.Namespace)
	if err != nil {
		return err
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
	for i := 0; i < len(r.Workloads); i++ {
		if len(r.Workloads[i]) == 0 {
			r.Workloads = append(r.Workloads[:i], r.Workloads[i+1:]...)
			i--
		}
	}
}
