/*
 * Copyright 2020-2021 VMware, Inc.
 * All Rights Reserved.
* Licensed under the Apache License, Version 2.0 (the "License");
* you may not use this file except in compliance with the License.
* You may obtain a copy of the License at
*   http://www.apache.org/licenses/LICENSE-2.0
* Unless required by applicable law or agreed to in writing, software
* distributed under the License is distributed on an "AS IS" BASIS,
* WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
* See the License for the specific language governing permissions and
* limitations under the License.
*/

package k8s

import (
	"reflect"
	"strings"
	"time"

	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/lib"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/objects"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/pkg/utils"
	"k8s.io/client-go/tools/cache"
	gwV2crd "sigs.k8s.io/gateway-api/pkg/client/clientset/versioned"
	gwV2Apiinformers "sigs.k8s.io/gateway-api/pkg/client/informers/externalversions"

	gwV2api "sigs.k8s.io/gateway-api/apis/v1beta1"
)

func NewgwV2ApiInformers(cs gwV2crd.Interface) {
	gwV2ApiInfomerFactory := gwV2Apiinformers.NewSharedInformerFactory(cs, time.Second*30)
	gwInformer := gwV2ApiInfomerFactory.Gateway().V1beta1().Gateways()
	gwClassInformer := gwV2ApiInfomerFactory.Gateway().V1beta1().GatewayClasses()
	//httpRouteInformer := gwV2ApiInfomerFactory.Gateway().V1beta1().HTTPRoutes()
	lib.AKOControlConfig().SetGatewayV2Informers(&lib.GwV2Informers{
		GatewayInformer:      gwInformer,
		GatewayClassInformer: gwClassInformer,
	})
}

func (c *AviController) SetupGatewayV2ApiEventHandlers(numWorkers uint32) {
	utils.AviLog.Infof("Setting up GatewayV2 Event handlers")
	informer := lib.AKOControlConfig().GatewayV2Informers()

	gatewayEventHandler := cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			if c.DisableSync {
				return
			}
			gw := obj.(*gwV2api.Gateway)
			if !isValidGateway(gw) {
				return
			}
			key := utils.Gateway + "/" + utils.ObjKey(gw)
			ok, resVer := objects.SharedResourceVerInstanceLister().Get(key)
			if ok && resVer.(string) == gw.ResourceVersion {
				utils.AviLog.Debugf("key : %s, msg: same resource version returning", key)
				return
			}
			namespace, _, _ := cache.SplitMetaNamespaceKey(utils.ObjKey(gw))
			bkt := utils.Bkt(namespace, numWorkers)
			c.workqueue[bkt].AddRateLimited(key)
			utils.AviLog.Debugf("key: %s, msg: ADD", key)
		},
		DeleteFunc: func(obj interface{}) {
			if c.DisableSync {
				return
			}
			gw := obj.(*gwV2api.Gateway)
			key := utils.Gateway + "/" + utils.ObjKey(gw)
			objects.SharedResourceVerInstanceLister().Delete(key)
			namespace, _, _ := cache.SplitMetaNamespaceKey(utils.ObjKey(gw))
			bkt := utils.Bkt(namespace, numWorkers)
			c.workqueue[bkt].AddRateLimited(key)
			utils.AviLog.Debugf("key: %s, msg: DELETE", key)
		},
		UpdateFunc: func(old, obj interface{}) {
			if c.DisableSync {
				return
			}
			oldGw := old.(*gwV2api.Gateway)
			gw := obj.(*gwV2api.Gateway)
			if !reflect.DeepEqual(oldGw.Spec, gw.Spec) || gw.GetDeletionTimestamp() != nil {
				if !isValidGateway(gw) {
					return
				}
				key := utils.Gateway + "/" + utils.ObjKey(gw)
				namespace, _, _ := cache.SplitMetaNamespaceKey(utils.ObjKey(gw))
				bkt := utils.Bkt(namespace, numWorkers)
				c.workqueue[bkt].AddRateLimited(key)
				utils.AviLog.Debugf("key: %s, msg: UPDATE", key)
			}
		},
	}
	informer.GatewayInformer.Informer().AddEventHandler(gatewayEventHandler)
}

func isValidGateway(gateway *gwV2api.Gateway) bool {
	spec := gateway.Spec
	//is associated with gateway class
	if spec.GatewayClassName == "" {
		utils.AviLog.Errorf("no gatewayclass found in gateway %+v", gateway.Name)
		return false
	}

	//has more than 1 listener
	if len(spec.Listeners) == 0 {
		utils.AviLog.Errorf("no listeners found in gateway %+v", gateway.Name)
		return false
	}

	//has 1 or none addresses
	if len(spec.Addresses) > 1 {
		utils.AviLog.Errorf("more than 1 gateway address found in gateway %+v", gateway.Name)
		return false
	}

	for _, listener := range spec.Listeners {
		if !isValidListener(gateway.Name, listener) {
			return false
		}
	}
	return true
}

func isValidListener(gwName string, listener gwV2api.Listener) bool {
	//has valid name
	if listener.Name == "" {
		utils.AviLog.Errorf("no listener name found in gateway %+v", gwName)
		return false
	}
	//hostname is not wildcard
	if listener.Hostname != nil && strings.Contains(string(*listener.Hostname), "*") {
		utils.AviLog.Errorf("listener hostname with wildcard found in gateway %+v", gwName)
		return false
	}
	//port and protocol valid

	//has valid TLS config
	if listener.TLS != nil {
		if *listener.TLS.Mode != "Terminate" || len(listener.TLS.CertificateRefs) == 0 {
			utils.AviLog.Errorf("tls mode/ref not valid %+v/%+v", gwName, listener.Name)
			return false
		}
	}
	return true
}
