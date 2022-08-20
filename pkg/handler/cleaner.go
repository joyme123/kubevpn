package handler

import (
	"context"
	"encoding/json"
	"strconv"

	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	v12 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/util/retry"

	"github.com/wencaiwulue/kubevpn/pkg/config"
	"github.com/wencaiwulue/kubevpn/pkg/dns"
)

var RollbackFuncList = make([]func(), 2)
var MapFunc = make(map[string]func())

func (r *ReverseOptions) Cleanup() {
	log.Infoln("prepare to exit, cleaning up")
	dns.CancelDNS()

	for _, function := range RollbackFuncList {
		if function != nil {
			function()
		}
	}

	RollbackFuncList = RollbackFuncList[0:0]

	err := r.dhcp.Release()
	if err != nil {
		log.Errorln(err)
	}

	cleanUpTrafficManagerIfRefCountIsZero(r.clientset, r.Namespace)
	log.Infoln("clean up successful")
}

// vendor/k8s.io/kubectl/pkg/polymorphichelpers/rollback.go:99
func updateServiceRefCount(serviceInterface v12.ServiceInterface, name string, increment int) {
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		service, err := serviceInterface.Get(context.TODO(), name, v1.GetOptions{})
		if err != nil {
			log.Errorf("update ref-count failed, increment: %d, error: %v", increment, err)
			return err
		}
		curCount := 0
		if ref := service.GetAnnotations()["ref-count"]; len(ref) > 0 {
			curCount, err = strconv.Atoi(ref)
		}
		p, _ := json.Marshal([]interface{}{
			map[string]interface{}{
				"op":    "replace",
				"path":  "/metadata/annotations/ref-count",
				"value": strconv.Itoa(curCount + increment),
			},
		})
		_, err = serviceInterface.Patch(context.TODO(), config.PodTrafficManager, types.JSONPatchType, p, v1.PatchOptions{})
		return err
	})
	if err != nil {
		log.Errorf("update ref count error, error: %v", err)
	} else {
		log.Infof("update ref count successfully")
	}
}

func cleanUpTrafficManagerIfRefCountIsZero(clientset *kubernetes.Clientset, namespace string) {
	updateServiceRefCount(clientset.CoreV1().Services(namespace), config.PodTrafficManager, -1)
	svc, err := clientset.CoreV1().Services(namespace).Get(context.TODO(), config.PodTrafficManager, v1.GetOptions{})
	if err != nil {
		log.Error(err)
		return
	}
	refCount, err := strconv.Atoi(svc.GetAnnotations()["ref-count"])
	if err != nil {
		log.Error(err)
		return
	}
	// if refCount is less than zero or equals to zero, means nobody is using this traffic pod, so clean it
	if refCount <= 0 {
		zero := int64(0)
		log.Infoln("refCount is zero, prepare to cleanup resource")
		deleteOptions := v1.DeleteOptions{GracePeriodSeconds: &zero}
		_ = clientset.CoreV1().ConfigMaps(namespace).Delete(context.TODO(), config.PodTrafficManager, deleteOptions)
		_ = clientset.CoreV1().Services(namespace).Delete(context.TODO(), config.PodTrafficManager, deleteOptions)
		_ = clientset.AppsV1().Deployments(namespace).Delete(context.TODO(), config.PodTrafficManager, deleteOptions)
	}
}
