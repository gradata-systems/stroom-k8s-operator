package v1

import corev1 "k8s.io/api/core/v1"

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// DatabaseServerRef contains either the details of a DatabaseServer resource, or the TCP connection details of
	// an external MySQL database
	// +kubebuilder:validation:Required
	DatabaseServerRef DatabaseServerRef `json:"databaseServerRef"`
	// +kubebuilder:default:="stroom"
	// +kubebuilder:validation:MinLength=1
	AppDatabaseName string `json:"appDatabaseName"`
	// +kubebuilder:default:="stats"
	// +kubebuilder:validation:MinLength=1
	StatsDatabaseName string `json:"statsDatabaseName"`
	// Override the Stroom configuration provided to each node
	ConfigMapRef ConfigMapRef `json:"configMapRef,omitempty"`
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

	// Additional environment variables provided to each NodeSet pod
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
	// Additional volumes to be mounted in each NodeSet pod
	ExtraVolumes      []corev1.Volume      `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`

	// Configures the mechanism that posts internal audit and logging to Stroom
	LogSender LogSenderSettings `json:"logSender,omitempty"`
}

type ConfigMapRef struct {
	Name     string `json:"name,omitempty"`
	ItemName string `json:"itemName,omitempty"`
}

func (in *ConfigMapRef) IsZero() bool {
	return in.Name == "" && in.ItemName == ""
}

type LogSenderSettings struct {
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled"`
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Override the container security context
	SecurityContext corev1.SecurityContext `json:"securityContext,omitempty"`
	// Configure an alternate destination for events to be shipped to. If omitted, events are posted to the local cluster.
	DestinationUrl string `json:"destinationUrl,omitempty"`
	// Name of the `Environment` to set in feed metadata. If omitted, the cluster name is used (converted to UPPERCASE).
	EnvironmentName string `json:"environmentName,omitempty"`
}
