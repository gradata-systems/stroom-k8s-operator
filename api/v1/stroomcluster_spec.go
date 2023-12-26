package v1

import (
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

// StroomClusterSpec defines the desired state of StroomCluster
type StroomClusterSpec struct {
	// +kubebuilder:validation:Required
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// DatabaseServerRef contains either the details of a DatabaseServer resource, or the TCP connection details of
	// an external MySQL database
	// +kubebuilder:validation:Required
	DatabaseServerRef DatabaseServerRef `json:"databaseServerRef"`
	// Name of the main Stroom database, usually `stroom`
	// +kubebuilder:default:="stroom"
	// +kubebuilder:validation:MinLength=1
	AppDatabaseName string `json:"appDatabaseName"`
	// Name of the statistics database, usually `stats`
	// +kubebuilder:default:="stats"
	// +kubebuilder:validation:MinLength=1
	StatsDatabaseName string `json:"statsDatabaseName"`
	// Override the Stroom configuration provided to each node, by providing the name of an existing `ConfigMap`
	// in the same namespace as the `StroomCluster`
	ConfigMapRef ConfigMapRef `json:"configMapRef,omitempty"`
	// Configures OpenID to enable operator components to query the Stroom API
	// +kubebuilder:validation:Required
	OpenId OpenIdConfiguration `json:"openId"`
	// +kubebuilder:validation:Required
	Ingress IngressSettings `json:"ingress"`
	// Pod management policy to use when deploying or scaling the StroomCluster
	PodManagementPolicy v1.PodManagementPolicyType `json:"podManagementPolicy,omitempty"`
	// Amount of time granted to nodes to drain their active tasks before being terminated
	// +kubebuilder:default:=60
	NodeTerminationPeriodSecs int64 `json:"nodeTerminationPeriodSecs"`
	// Delete Stroom node `PersistentVolumeClaim`s in accordance with this policy
	VolumeClaimDeletePolicy VolumeClaimDeletePolicy `json:"volumeClaimDeletePolicy,omitempty"`

	// Each NodeSet is a functional grouping of Stroom nodes with a particular role, within the cluster.
	// It is recommended two NodeSets should be provided: one for storing and processing data and a separate one for
	// serving the Stroom front-end.
	// +kubebuilder:validation:MinItems=1
	NodeSets []NodeSet `json:"nodeSets"`

	// Additional Java Virtual Machine (JVM) options to use. Example of a valid entry: `-Xms1g`
	ExtraJvmOpts []string `json:"extraJvmOpts,omitempty"`
	// Additional environment variables provided to each NodeSet pod
	ExtraEnv []corev1.EnvVar `json:"extraEnv,omitempty"`
	// Additional volumes to be mounted in each NodeSet pod
	ExtraVolumes      []corev1.Volume      `json:"extraVolumes,omitempty"`
	ExtraVolumeMounts []corev1.VolumeMount `json:"extraVolumeMounts,omitempty"`

	// Configures the mechanism that posts internal audit and logging to Stroom
	LogSender LogSenderSettings `json:"logSender,omitempty"`
}

type ConfigMapRef struct {
	// Name of the `ConfigMap`
	Name string `json:"name,omitempty"`
	// ConfigMap key containing the Stroom configuration data
	ItemName string `json:"itemName,omitempty"`
}

func (in *ConfigMapRef) IsZero() bool {
	return in.Name == "" && in.ItemName == ""
}

type LogSenderSettings struct {
	// If `true`, Stroom internal audit and application logs are sent to the Stroom `Ingress` for ingestion
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
	// Name of the `System` to set in feed metadata
	SystemName string                      `json:"systemName,omitempty"`
	Resources  corev1.ResourceRequirements `json:"resources,omitempty"`
}

func (in *LogSenderSettings) IsZero() bool {
	return !in.Enabled
}
