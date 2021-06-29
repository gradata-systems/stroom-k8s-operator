package v1

import corev1 "k8s.io/api/core/v1"

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	ExtraEnv        []corev1.EnvVar   `json:"extraEnv,omitempty"`
	// +kubebuilder:validation:MinLength=1
	ConfigMapName string `json:"configMapName"`
	// +kubebuilder:validation:Required
	AppDatabaseRef DatabaseRef `json:"appDatabaseRef"`
	// +kubebuilder:validation:Required
	StatsDatabaseRef DatabaseRef `json:"statsDatabaseRef"`
	// +kubebuilder:validation:Required
	Ingress IngressSettings `json:"ingress"`

	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`
}
