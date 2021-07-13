package v1

import corev1 "k8s.io/api/core/v1"

// DatabaseServerSpec defines the desired state of DatabaseServer
type DatabaseServerSpec struct {
	Image           Image             `json:"image,omitempty"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Names of the databases to be created on startup (if they don't already exist)
	DatabaseNames []string `json:"databaseNames"`
	// Any additional configuration lines to append to the MySQL server configuration file `/etc/my.cnf`
	AdditionalConfig []string                         `json:"additionalConfig,omitempty"`
	Resources        corev1.ResourceRequirements      `json:"resources"`
	VolumeClaim      corev1.PersistentVolumeClaimSpec `json:"volumeClaim"`
	// Configures backup destination, frequency and database names
	Backup                BackupSettings            `json:"backup,omitempty"`
	ReadinessProbeTimings ProbeTimings              `json:"readinessProbeTimings,omitempty"`
	LivenessProbeTimings  ProbeTimings              `json:"livenessProbeTimings,omitempty"`
	PodAnnotations        map[string]string         `json:"podAnnotations,omitempty"`
	PodSecurityContext    corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	SecurityContext       corev1.SecurityContext    `json:"securityContext,omitempty"`
	NodeSelector          map[string]string         `json:"nodeSelector,omitempty"`
	Tolerations           []corev1.Toleration       `json:"tolerations,omitempty"`
	Affinity              corev1.Affinity           `json:"affinity,omitempty"`
}
