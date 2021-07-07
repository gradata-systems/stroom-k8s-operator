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

	// Options for automatically adjusting the number of server tasks based on pod resource metrics
	TaskAutoScaleOptions TaskAutoScaleOptions `json:"taskAutoScaleOptions,omitempty"`
}

type NodeRole string

const (
	Processing NodeRole = "Processing"
	Frontend            = "Frontend"
)

type TaskAutoScaleOptions struct {
	// Whether to auto-scale node task limits
	// +kubebuilder:validation:Default=false
	Enabled bool `json:"enabled,omitempty"`

	// How often (in minutes) adjustments are made to the number of Stroom node tasks
	// +kubebuilder:validation:Default=1
	AdjustmentIntervalMins int `json:"adjustmentIntervalMins,omitempty"`

	// Sliding window (in minutes) over which to calculate CPU usage vs. the threshold parameters
	// +kubebuilder:validation:Default=1
	MetricsSlidingWindowMins int `json:"metricsSlidingWindowMins,omitempty"`

	// Minimum CPU usage threshold before the number of tasks is adjust upwards
	// +kubebuilder:validation:Default=50
	MinCpuPercent int `json:"minCpuPercent,omitempty"`

	// Maximum CPU usage threshold before the number of tasks is adjusted downwards
	// +kubebuilder:validation:Default=90
	MaxCpuPercent int `json:"maxCpuPercent,omitempty"`

	// Minimum number of tasks auto-scaler may set the node limit to
	// +kubebuilder:validation:Default=1
	MinTaskLimit int `json:"minTaskLimit,omitempty"`

	// Maximum number of tasks auto-scaler may set the node limit to
	// +kubebuilder:validation:Default=20
	MaxTaskLimit int `json:"maxTaskLimit,omitempty"`

	// Number of tasks to add/subtract each adjustment interval, based on usage
	// +kubebuilder:validation:Default=1
	StepAmount int `json:"stepAmount"`
}
