/*
 * Copyright 2019-2020 VMware, Inc.
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

package objects

import (
	"sync"

	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/pkg/utils"
)

var gwv2lister *GWV2Lister
var gwv2once sync.Once

// This file builds cache relations for all gateway V2 API objects.

func GatewayV2Lister() *GWV2Lister {
	gwv2once.Do(func() {
		gwv2lister = &GWV2Lister{
			GwClassGWStore:   NewObjectMapStore(),
			GwGwClassStore:   NewObjectMapStore(),
			GwListenersStore: NewObjectMapStore(),
			RouteGWStore:     NewObjectMapStore(),
			GwRouteStore:     NewObjectMapStore(),
		}
	})
	return gwv2lister
}

type GWV2Lister struct {
	GWV2Lock sync.RWMutex

	// gwclass -> [ns1/gw1, ns1/gw2, ns2/gw3]
	GwClassGWStore *ObjectMapStore

	// nsX/gw1 -> gwclass
	GwGwClassStore *ObjectMapStore

	// the protocol and port mapped here are of the gateway listener config
	// that has the appropriate labels
	// nsX/gw1 -> [proto1/port1, proto2/port2]
	GwListenersStore *ObjectMapStore

	RouteGWStore *ObjectMapStore
	GwRouteStore *ObjectMapStore
}

// Gateway <-> GatewayClass
func (v *GWV2Lister) GetGWclassToGateways(gwclass string) (bool, []string) {
	found, gatewayList := v.GwClassGWStore.Get(gwclass)
	if !found {
		return false, make([]string, 0)
	}
	return true, gatewayList.([]string)
}

func (v *GWV2Lister) GetGatewayToGWclass(gateway string) (bool, string) {
	found, gwClass := v.GwGwClassStore.Get(gateway)
	if !found {
		return false, ""
	}
	return true, gwClass.(string)
}

func (v *GWV2Lister) UpdateGatewayGWclassMappings(gateway, gwclass string) {
	v.GWV2Lock.Lock()
	defer v.GWV2Lock.Unlock()
	_, gatewayList := v.GetGWclassToGateways(gwclass)
	if !utils.HasElem(gatewayList, gateway) {
		gatewayList = append(gatewayList, gateway)
	}
	v.GwClassGWStore.AddOrUpdate(gwclass, gatewayList)
	v.GwGwClassStore.AddOrUpdate(gateway, gwclass)
}

func (v *GWV2Lister) RemoveGatewayGWclassMappings(gateway string) bool {
	v.GWV2Lock.Lock()
	defer v.GWV2Lock.Unlock()
	found, gwclass := v.GetGatewayToGWclass(gateway)
	if !found {
		return false
	}

	if found, gatewayList := v.GetGWclassToGateways(gwclass); found && utils.HasElem(gatewayList, gateway) {
		gatewayList = utils.Remove(gatewayList, gateway)
		if len(gatewayList) == 0 {
			v.GwClassGWStore.Delete(gwclass)
			return true
		}
		v.GwClassGWStore.AddOrUpdate(gwclass, gatewayList)
		return true
	}
	return false
}

// Gateway <-> Listeners
func (v *GWV2Lister) GetGWListeners(gateway string) (bool, []string) {
	found, listeners := v.GwListenersStore.Get(gateway)
	if !found {
		return false, make([]string, 0)
	}
	return true, listeners.([]string)
}

func (v *GWV2Lister) UpdateGWListeners(gateway string, listeners []string) {
	v.GwListenersStore.AddOrUpdate(gateway, listeners)
}

func (v *GWV2Lister) DeleteGWListeners(gateway string) bool {
	success := v.GwListenersStore.Delete(gateway)
	return success
}
