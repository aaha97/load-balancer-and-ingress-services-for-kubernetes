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

package nodes

import (
	"context"
	"strings"

	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/internal/lib"
	"github.com/vmware/load-balancer-and-ingress-services-for-kubernetes/pkg/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type ListenerModel struct {
	Name     string
	Port     int32
	Protocol string
	TLSType  *string
	CertRefs []string

	//no wildcards
	Hostname *string

	//kind/namespace/name
	//TODO add selector support
	AllowedRoute string
}
type GatewayModel struct {
	Name        string
	Namespace   string
	Listeners   []ListenerModel
	Address     string
	AddressType string
}

func (o *AviObjectGraph) BuildGatewayV2(gateway GatewayModel, key string) {
	o.Lock.Lock()
	defer o.Lock.Unlock()
	var vsNode *AviEvhVsNode

	vsNode = o.ConstructGwV2ParentVsNode(gateway, key)

	o.AddModelNode(vsNode)
	utils.AviLog.Infof("key: %s, msg: checksum for AVI VS object %v", key, vsNode.GetCheckSum())
}

func (o *AviObjectGraph) ConstructGwV2ParentVsNode(gateway GatewayModel, key string) *AviEvhVsNode {

	//Set name
	vsName := lib.GetGWParentName(gateway.Namespace, gateway.Name)

	avi_vs_meta := &AviEvhVsNode{
		Name:               vsName,
		Tenant:             lib.GetTenant(),
		ServiceEngineGroup: lib.GetSEGName(),
		ApplicationProfile: utils.DEFAULT_L7_APP_PROFILE,
		NetworkProfile:     utils.DEFAULT_TCP_NW_PROFILE,
		EVHParent:          true,
		SharedVS:           true,
		VrfContext:         lib.GetVrf(),
		ServiceMetadata: lib.ServiceMetadataObj{
			Gateway: gateway.Namespace + "/" + gateway.Name,
		},
	}

	//get port/protocol from listener

	var portProtocols []AviPortHostProtocol
	for _, listener := range gateway.Listeners {
		pp := AviPortHostProtocol{Port: listener.Port, Protocol: listener.Protocol}
		//TLS config on listener is present
		if listener.TLSType != nil && len(listener.CertRefs) > 0 {
			pp.EnableSSL = true
		}
		portProtocols = append(portProtocols, pp)
	}
	avi_vs_meta.PortProto = portProtocols

	//Create tls nodes
	tlsNodes := BuildTLSNodesForGateway(gateway.Listeners, key)
	if len(tlsNodes) > 0 {
		avi_vs_meta.SSLKeyCertRefs = tlsNodes
	}

	//TODO
	//avi_vs_meta.SSLKeyCertAviRef = []string{}

	//Create vsvipNode
	vsvipNode := BuildVsVipNodeForGateway(gateway, avi_vs_meta.Name)
	avi_vs_meta.VSVIPRefs = []*AviVSVIPNode{vsvipNode}

	return avi_vs_meta
}

func BuildTLSNodesForGateway(listeners []ListenerModel, key string) []*AviTLSKeyCertNode {
	var tlsNodes []*AviTLSKeyCertNode
	cs := utils.GetInformers().ClientSet
	for _, listener := range listeners {
		if listener.TLSType != nil {
			for _, certRef := range listener.CertRefs {
				certRefSlice := strings.Split(certRef, "/")
				ns, name := certRefSlice[1], certRefSlice[2]
				secretObj, err := cs.CoreV1().Secrets(ns).Get(context.TODO(), name, metav1.GetOptions{})
				if err != nil || secretObj == nil {
					utils.AviLog.Warnf("key: %s, msg: secret: %s has been deleted, err: %s", key, name, err)
					continue
				}
				tlsNode := TLSNodeFromSecret(secretObj, *listener.Hostname, certRefSlice[2], key)
				tlsNodes = append(tlsNodes, tlsNode)
			}
		}
	}
	return tlsNodes
}

func TLSNodeFromSecret(secretObj *v1.Secret, hostname, certName, key string) *AviTLSKeyCertNode {
	keycertMap := secretObj.Data
	tlscert, ok := keycertMap[utils.K8S_TLS_SECRET_CERT]
	if !ok {
		utils.AviLog.Infof("key: %s, msg: certificate not found for secret: %s", key, secretObj.Name)
	}
	tlskey, ok := keycertMap[utils.K8S_TLS_SECRET_KEY]
	if !ok {

		utils.AviLog.Infof("key: %s, msg: key not found for secret: %s", key, secretObj.Name)
	}
	tlsNode := &AviTLSKeyCertNode{
		Name:   lib.GetTLSKeyCertNodeName("", hostname, certName),
		Tenant: lib.GetTenant(),
		Type:   lib.CertTypeVS,
		Key:    tlskey,
		Cert:   tlscert,
	}
	return tlsNode
}

func BuildVsVipNodeForGateway(gateway GatewayModel, vsName string) *AviVSVIPNode {
	vsvipNode := &AviVSVIPNode{
		Name:        lib.GetVsVipName(vsName),
		Tenant:      lib.GetTenant(),
		VrfContext:  lib.GetVrf(),
		VipNetworks: lib.GetVipNetworkList(),
	}

	if gateway.Address != "" {
		vsvipNode.IPAddress = gateway.Address
	}
	return vsvipNode
}
