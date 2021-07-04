package v1

import corev1 "k8s.io/api/core/v1"

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Additional environment variables provided to NodeSet pods
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
	// +kubebuilder:validation:Required
	AppDatabaseRef DatabaseRef `json:"appDatabaseRef"`
	// +kubebuilder:validation:Required
	StatsDatabaseRef DatabaseRef `json:"statsDatabaseRef"`
	// +kubebuilder:validation:Required
	Ingress IngressSettings `json:"ingress"`
	// Amount of time granted to nodes to drain their active tasks before being terminated
	// +kubebuilder:validation:Default=60
	NodeTerminationPeriodSecs int64 `json:"nodeTerminationPeriodSecs"`

	// Each NodeSet is a functional grouping of Stroom nodes with a particular role, within the cluster.
	// It is recommended two NodeSets should be provided: one for storing and processing data and a separate one for
	// serving the Stroom front-end.
	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`
}
