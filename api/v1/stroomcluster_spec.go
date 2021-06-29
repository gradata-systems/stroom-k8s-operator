package v1

import corev1 "k8s.io/api/core/v1"

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	Image             Image             `json:"image,omitempty"`
	ImagePullPolicy   corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	MaxClientBodySize string            `json:"maxClientBodySize,omitempty"`
	ExtraEnv          []corev1.EnvVar   `json:"extraEnv,omitempty"`
	ConfigMapName     string            `json:"configMapName"`
	AppDatabaseRef    DatabaseRef       `json:"appDatabaseRef"`
	StatsDatabaseRef  DatabaseRef       `json:"statsDatabaseRef"`
	Ingress           IngressSettings   `json:"ingress"`

	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`
}
