package v1

import corev1 "k8s.io/api/core/v1"

// DatabaseServerSpec defines the desired state of DatabaseServer
type DatabaseServerSpec struct {
	Image           Image             `json:"image"`
	ImagePullPolicy corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
	// Names of the databases to be created on startup (if they don't already exist)
	// +kubebuilder:validation:items:Pattern=`^[A-Za-z0-9_]{1,64}$`
	DatabaseNames []string `json:"databaseNames"`
	// Any additional configuration lines to append to the `[mysqld]` section of the MySQL server configuration file
	// `/etc/my.cnf`, each a single option, such as `max_connections=500`. Options that the operator sets, or that
	// would give access to the server's files or bypass authentication, aren't allowed.
	// +kubebuilder:validation:MaxItems=200
	// +kubebuilder:validation:items:MaxLength=1024
	// +kubebuilder:validation:items:Pattern=`^[A-Za-z][A-Za-z0-9_-]*(\s*=.*)?$`
	// +kubebuilder:validation:XValidation:rule="self.all(line, !(line.split('=')[0].trim().lowerAscii().replace('-', '_') in ['datadir', 'port', 'user', 'secure_file_priv', 'local_infile', 'plugin_dir', 'plugin_load', 'plugin_load_add', 'init_file', 'skip_grant_tables', 'general_log_file', 'slow_query_log_file', 'log_error', 'pid_file', 'socket', 'tmpdir']))",message="additionalConfig can't set datadir, port, user, secure_file_priv, local_infile, plugin_dir, plugin_load, plugin_load_add, init_file, skip_grant_tables, general_log_file, slow_query_log_file, log_error, pid_file, socket or tmpdir"
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
