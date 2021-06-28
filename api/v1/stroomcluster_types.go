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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	Image             Image             `json:"image,omitempty"`
	ImagePullPolicy   corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	MaxClientBodySize string            `json:"maxClientBodySize,omitempty"`
	ExtraEnv          []corev1.EnvVar   `json:"extraEnv,omitempty"`
	ConfigMapName     string            `json:"configMapName"`
	AppDatabaseRef    DatabaseRef       `json:"appDatabaseRef"`
	StatsDatabaseRef  DatabaseRef       `json:"statsDatabaseRef"`

	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`
}

type IngressSettings struct {
	HostName   string `json:"hostName"`
	SecretName string `json:"secretName"`
	ClassName  string `json:"className,omitempty"`
}

type DatabaseRef struct {
	// If specified, point to an operator-managed DatabaseServer object
	DatabaseServerRef ResourceRef `json:"databaseServerRef,omitempty"`

	// Alternatively, if the following parameters are provided, point directly to a DB by its TCP address.
	// This allows external database instances to be used in place of an operator-managed one.
	ConnectionSpec DatabaseAddress `json:"connectionSpec,omitempty"`

	DatabaseName string `json:"databaseName"`
}

type DatabaseAddress struct {
	Address    string `json:"address,omitempty"`
	Port       int32  `json:"port,omitempty"`
	SecretName string `json:"secretName,omitempty"`
}

// StroomClusterStatus defines the observed state of StroomCluster
type StroomClusterStatus struct {
	Nodes []string `json:"nodes,omitempty"`
}

type VolumeClaimDeletePolicy string

const (
	DeleteOnScaledownAndClusterDeletionPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownAndClusterDeletion"
	DeleteOnScaledownOnlyPolicy                                       = "DeleteOnScaledownOnly"
)

type NodeSet struct {
	Name                    string                           `json:"name"`
	Count                   int32                            `json:"count"`
	Role                    NodeRole                         `json:"role,omitempty"`
	LocalDataVolumeClaim    corev1.PersistentVolumeClaimSpec `json:"localDataVolumeClaim"`
	SharedDataVolume        corev1.VolumeSource              `json:"sharedDataVolume,omitempty"`
	VolumeClaimDeletePolicy VolumeClaimDeletePolicy          `json:"volumeClaimDeletePolicy,omitempty"`
	Ingress                 IngressSettings                  `json:"ingress"`
	Resources               corev1.ResourceRequirements      `json:"resources"`
	StartupProbeTimings     ProbeTimings                     `json:"startupProbeTimings,omitempty"`
	LivenessProbeTimings    ProbeTimings                     `json:"livenessProbeTimings,omitempty"`
	JavaOpts                string                           `json:"javaOpts,omitempty"`
	PodAnnotations          map[string]string                `json:"podAnnotations,omitempty"`
	PodSecurityContext      corev1.PodSecurityContext        `json:"podSecurityContext,omitempty"`
	SecurityContext         corev1.SecurityContext           `json:"securityContext,omitempty"`
	NodeSelector            map[string]string                `json:"nodeSelector,omitempty"`
	Tolerations             []corev1.Toleration              `json:"tolerations,omitempty"`
	Affinity                corev1.Affinity                  `json:"affinity,omitempty"`
}

type NodeRole string

const (
	Processing NodeRole = "Processing"
	Frontend            = "Frontend"
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
