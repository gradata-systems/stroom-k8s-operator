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
	Image             string            `json:"image,omitempty"`
	ImagePullPolicy   corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	MaxClientBodySize string            `json:"maxClientBodySize,omitempty"`
	ExtraEnv          []corev1.EnvVar   `json:"extraEnv,omitempty"`
	ConfigMapName     string            `json:"configMapName"`
	AppDatabaseRef    DatabaseRef       `json:"appDatabaseRef"`
	StatsDatabaseRef  DatabaseRef       `json:"statsDatabaseRef"`
	Ingress           struct {
		HostName   string `json:"hostName"`
		SecretName string `json:"secretName"`
		ClassName  string `json:"className,omitempty"`
	} `json:"ingress"`

	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`
}

type DatabaseRef struct {
	// If Namespace and Name are provided, point to the associated operator-managed DatabaseServer object
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name,omitempty"`

	// Alternatively, if the following parameters are provided, point directly to this DNS name.
	// This allows external database instances to be used in place of an operator-generated one.
	Address    string `json:"serviceName,omitempty"`
	Port       int32  `json:"port,omitempty"`
	SecretName string `json:"secretName,omitempty"`

	DatabaseName string `json:"databaseName"`
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
	SharedDataVolume        corev1.VolumeSource              `json:"sharedDataVolume"`
	VolumeClaimDeletePolicy VolumeClaimDeletePolicy          `json:"volumeClaimDeletePolicy,omitempty"`
	StartupProbeTimings     ProbeTimings                     `json:"startupProbeTimings,omitempty"`
	LivenessProbeTimings    ProbeTimings                     `json:"livenessProbeTimings,omitempty"`
	Resources               corev1.ResourceRequirements      `json:"resources,omitempty"`
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

type ProbeTimings struct {
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int32 `json:"timeoutSeconds,omitempty"`
	PeriodSeconds       int32 `json:"periodSeconds,omitempty"`
	SuccessThreshold    int32 `json:"successThreshold,omitempty"`
	FailureThreshold    int32 `json:"failureThreshold,omitempty"`
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
