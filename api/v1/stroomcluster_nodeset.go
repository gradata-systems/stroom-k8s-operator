package v1

import corev1 "k8s.io/api/core/v1"

type NodeSet struct {
	// Unique identity of the NodeSet. Should be a short name like "prod".
	Name string `json:"name"`
	// Number of replicas (discrete Stroom nodes) in the NodeSet
	// +kubebuilder:validation:Minimum=1
	Count                 int32                            `json:"count"`
	Role                  NodeRole                         `json:"role,omitempty"`
	LocalDataVolumeClaim  corev1.PersistentVolumeClaimSpec `json:"localDataVolumeClaim"`
	SharedDataVolume      corev1.VolumeSource              `json:"sharedDataVolume"`
	Resources             corev1.ResourceRequirements      `json:"resources"`
	IngressEnabled        bool                             `json:"ingressEnabled,omitempty"`
	ReadinessProbeTimings ProbeTimings                     `json:"readinessProbeTimings,omitempty"`
	LivenessProbeTimings  ProbeTimings                     `json:"livenessProbeTimings,omitempty"`
	PodAnnotations        map[string]string                `json:"podAnnotations,omitempty"`
	PodSecurityContext    corev1.PodSecurityContext        `json:"podSecurityContext,omitempty"`
	SecurityContext       corev1.SecurityContext           `json:"securityContext,omitempty"`
	NodeSelector          map[string]string                `json:"nodeSelector,omitempty"`
	Tolerations           []corev1.Toleration              `json:"tolerations,omitempty"`
	Affinity              corev1.Affinity                  `json:"affinity,omitempty"`
}

type NodeRole string

const (
	ProcessingNodeRole NodeRole = "ProcessingNodeRole"
	FrontendNodeRole            = "FrontendNodeRole"
)
