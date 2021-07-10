/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	StroomClusterLabel = "stroom/cluster"
	NodeSetLabel       = "stroom/nodeSet"
)

// StroomClusterStatus defines the observed state of StroomCluster
type StroomClusterStatus struct {
	Nodes []string `json:"nodes,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// StroomCluster is the Schema for the stroomclusters API
type StroomCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StroomClusterSpec   `json:"spec,omitempty"`
	Status StroomClusterStatus `json:"status,omitempty"`
}

func (in *StroomCluster) GetBaseName() string {
	return fmt.Sprintf("stroom-%v", in.Name)
}

func (in *StroomCluster) GetStaticContentConfigMapName() string {
	return fmt.Sprintf("%v-static-content", in.GetBaseName())
}

func (in *StroomCluster) GetNodeSetName(nodeSet *NodeSet) string {
	return fmt.Sprintf("stroom-%v-node-%v", in.Name, nodeSet.Name)
}

func (in *StroomCluster) GetLogSenderConfigMapName() string {
	return fmt.Sprintf("stroom-%v-log-sender", in.Name)
}

func (in *StroomCluster) GetLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "stroom",
		"app.kubernetes.io/component": "stroom-cluster",
		StroomClusterLabel:            in.Name,
	}
}

func (in *StroomCluster) GetNodeSetSelectorLabels(nodeSet *NodeSet) map[string]string {
	return map[string]string{
		StroomClusterLabel: in.Name,
		NodeSetLabel:       nodeSet.Name,
	}
}

func (in *StroomCluster) GetNodeSetServiceName(nodeSet *NodeSet) string {
	return fmt.Sprintf("%v-http", in.GetNodeSetName(nodeSet))
}

func (in *StroomCluster) GetDatafeedUrl() string {
	return fmt.Sprintf("https://%v/stroom/datafeeddirect", in.Spec.Ingress.HostName)
}

func (in *StroomCluster) IsBeingDeleted() bool {
	return !in.ObjectMeta.DeletionTimestamp.IsZero()
}

//+kubebuilder:object:root=true

// StroomClusterList contains a list of StroomCluster
type StroomClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StroomCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StroomCluster{}, &StroomClusterList{})
}
