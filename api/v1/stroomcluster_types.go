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
	AppDatabase       DatabaseRef       `json:"appDatabase"`
	StatsDatabase     DatabaseRef       `json:"statsDatabase"`

	// +kubebuilder:validation:MinItems=1
	NodeSets []StroomNode `json:"nodeSets"`
}

type DatabaseRef struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace,omitempty"`
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

type StroomNode struct {
	Name                    string                           `json:"name"`
	Count                   uint                             `json:"count"`
	Role                    StroomNodeRole                   `json:"role,omitempty"`
	LocalDataVolumeClaim    corev1.PersistentVolumeClaimSpec `json:"localDataVolumeClaim"`
	SharedDataVolume        corev1.VolumeSource              `json:"sharedDataVolume"`
	VolumeClaimDeletePolicy VolumeClaimDeletePolicy          `json:"volumeClaimDeletePolicy,omitempty"`
	StartupProbe            corev1.Probe                     `json:"startupProbe,omitempty"`
	LivenessProbe           corev1.Probe                     `json:"livenessProbe,omitempty"`
	Resources               corev1.ResourceRequirements      `json:"resources,omitempty"`
	JavaOpts                string                           `json:"javaOpts,omitempty"`
	PodAnnotations          map[string]string                `json:"podAnnotations,omitempty"`
	PodSecurityContext      corev1.SecurityContext           `json:"podSecurityContext,omitempty"`
	SecurityContext         corev1.PodSecurityContext        `json:"securityContext,omitempty"`
	NodeSelector            map[string]string                `json:"nodeSelector,omitempty"`
	Tolerations             []corev1.Toleration              `json:"tolerations,omitempty"`
	Affinity                corev1.Affinity                  `json:"affinity,omitempty"`
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
