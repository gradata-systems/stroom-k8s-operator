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

// DatabaseServerStatus defines the observed state of DatabaseServer
type DatabaseServerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// DatabaseServer is the Schema for the databases API
type DatabaseServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   *DatabaseServerSpec   `json:"spec,omitempty"`
	Status *DatabaseServerStatus `json:"status,omitempty"`

	// Set by the controller when a StroomCluster binds to the DatabaseServer.
	// This is used to prevent the DatabaseServer from being deleted while its paired StroomCluster still exists.
	// +optional
	StroomClusterRef ResourceRef `json:"stroomClusterRef,omitempty"`
}

// GetBaseName creates a name incorporating the name of the database. For Example: stroom-prod-db
func (in *DatabaseServer) GetBaseName() string {
	return fmt.Sprintf("stroom-%v-db", in.Name)
}

func (in *DatabaseServer) GetLabels() map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      "stroom",
		"app.kubernetes.io/component": "database-server",
		"app.kubernetes.io/instance":  in.Name,
	}
}

func (in *DatabaseServer) GetServiceName() string {
	return fmt.Sprintf("%v-headless", in.GetBaseName())
}

func (in *DatabaseServer) GetSecretName() string {
	return fmt.Sprintf("%v", in.GetBaseName())
}

func (in *DatabaseServer) GetConfigMapName() string {
	return fmt.Sprintf("%v", in.GetBaseName())
}

func (in *DatabaseServer) GetInitConfigMapName() string {
	return fmt.Sprintf("%v-init", in.GetBaseName())
}

func (in *DatabaseServer) IsBeingDeleted() bool {
	return !in.ObjectMeta.DeletionTimestamp.IsZero()
}

//+kubebuilder:object:root=true

// DatabaseServerList contains a list of DatabaseServer
type DatabaseServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseServer `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseServer{}, &DatabaseServerList{})
}
