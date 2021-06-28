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

// DatabaseServerSpec defines the desired state of DatabaseServer
type DatabaseServerSpec struct {
	Image                 Image                            `json:"image,omitempty"`
	ImagePullPolicy       corev1.PullPolicy                `json:"imagePullPolicy,omitempty"`
	DatabaseNames         []string                         `json:"databaseNames"`
	AdditionalConfig      []string                         `json:"additionalConfig,omitempty"`
	Resources             corev1.ResourceRequirements      `json:"resources"`
	VolumeClaim           corev1.PersistentVolumeClaimSpec `json:"volumeClaim"`
	ReadinessProbeTimings ProbeTimings                     `json:"readinessProbeTimings,omitempty"`
	LivenessProbeTimings  ProbeTimings                     `json:"livenessProbeTimings,omitempty"`
	PodAnnotations        map[string]string                `json:"podAnnotations,omitempty"`
	PodSecurityContext    corev1.PodSecurityContext        `json:"podSecurityContext,omitempty"`
	SecurityContext       corev1.SecurityContext           `json:"securityContext,omitempty"`
	NodeSelector          map[string]string                `json:"nodeSelector,omitempty"`
	Tolerations           []corev1.Toleration              `json:"tolerations,omitempty"`
	Affinity              corev1.Affinity                  `json:"affinity,omitempty"`
}

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

	Spec   DatabaseServerSpec   `json:"spec,omitempty"`
	Status DatabaseServerStatus `json:"status,omitempty"`

	// Set by the controller when a StroomCluster binds to the DatabaseServer.
	// This is used to prevent the DatabaseServer from being deleted while its paired StroomCluster still exists.
	StroomClusterRef ResourceRef `json:"stroomClusterRef,omitempty"`
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
