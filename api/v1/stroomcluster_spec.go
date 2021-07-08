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
	DatabaseServerRef DatabaseServerRef `json:"databaseServerRef"`
	// +kubebuilder:default:="stroom"
	// +kubebuilder:validation:MinLength=1
	AppDatabaseName string `json:"appDatabaseName"`
	// +kubebuilder:default:="stats"
	// +kubebuilder:validation:MinLength=1
	StatsDatabaseName string `json:"statsDatabaseName"`
	// +kubebuilder:validation:Required
	Ingress IngressSettings `json:"ingress"`
	// Amount of time granted to nodes to drain their active tasks before being terminated
	// +kubebuilder:default:=60
	NodeTerminationPeriodSecs int64                   `json:"nodeTerminationPeriodSecs"`
	VolumeClaimDeletePolicy   VolumeClaimDeletePolicy `json:"volumeClaimDeletePolicy,omitempty"`

	// Each NodeSet is a functional grouping of Stroom nodes with a particular role, within the cluster.
	// It is recommended two NodeSets should be provided: one for storing and processing data and a separate one for
	// serving the Stroom front-end.
	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`

	// Configures the mechanism that posts internal audit and logging to Stroom
	LogSender LogSenderSettings `json:"logSender,omitempty"`
}

type LogSenderSettings struct {
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled"`
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// PersistentVolumeClaim providing access to the target Stroom node `logs` directory
	VolumeClaim corev1.PersistentVolumeClaimVolumeSource `json:"volumeClaim"`
	// Override the container security context
	PodSecurityContext corev1.PodSecurityContext `json:"securityContext,omitempty"`
	// Cron schedule that determines when the job should run
	Schedule string `json:"schedule"`
	// Configure an alternate destination for events to be shipped to. If omitted, events are posted to the local cluster.
	DestinationUrl string `json:"destinationUrl,omitempty"`
	// Name of the `Environment` to set in feed metadata. If omitted, the cluster name is used (converted to UPPERCASE).
	EnvironmentName string `json:"environmentName,omitempty"`
}
