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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	Name  string `json:"name"`
	Image struct {
		Repository string `json:"repository,omitempty"`
		Tag        string `json:"tag,omitempty"`
	} `json:"image,omitempty"`
	MaxClientBodySize string      `json:"maxClientBodySize,omitempty"`
	ExtraEnv          []v1.EnvVar `json:"extraEnv,omitempty"`
	AppDatabase       DatabaseRef `json:"appDatabase"`
	StatsDatabase     DatabaseRef `json:"statsDatabase"`

	// +kubebuilder:validation:MinItems=1
	NodeSets []StroomNode `json:"nodeSets"`
}

type DatabaseRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// StroomClusterStatus defines the observed state of StroomCluster
type StroomClusterStatus struct {
	Nodes []string `json:"nodes,omitempty"`
}

type StroomNode struct {
	Name      string         `json:"name"`
	Count     uint           `json:"count"`
	Role      StroomNodeRole `json:"role,omitempty"`
	LocalData struct {
		VolumeClaim v1.PersistentVolumeClaimSpec `json:"volumeClaim,omitempty"`
	} `json:"localData,omitempty"`
	SharedData struct {
		Volume v1.VolumeSource `json:"volume,omitempty"`
	} `json:"sharedData,omitempty"`
	StartupProbe       v1.Probe                `json:"startupProbe,omitempty"`
	LivenessProbe      v1.Probe                `json:"livenessProbe,omitempty"`
	Resources          v1.ResourceRequirements `json:"resources,omitempty"`
	JavaOpts           string                  `json:"javaOpts,omitempty"`
	PodAnnotations     map[string]string       `json:"podAnnotations"`
	PodSecurityContext v1.SecurityContext      `json:"podSecurityContext"`
}

type StroomNodeRole string

const (
	Processing StroomNodeRole = "Processing"
	Frontend                  = "Frontend"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// StroomCluster is the Schema for the stroomclusters API
type StroomCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StroomClusterSpec   `json:"spec,omitempty"`
	Status StroomClusterStatus `json:"status,omitempty"`
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
