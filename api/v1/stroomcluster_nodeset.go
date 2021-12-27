package v1

import corev1 "k8s.io/api/core/v1"

type NodeSet struct {
	// Name uniquely identifies the NodeSet within a StroomCluster. Should be a short name like "prod".
	//
	// The NodeSet name determines the name of the Stroom node, which follows the format:
	// `stroom-<cluster name>-node-<nodeset name>-<pod ordinal>`.
	//
	// For example, the first Stroom node in StroomCluster `dev`, NodeSet `data` will be named:
	// `stroom-dev-node-data-0`.
	Name string `json:"name"`
	// Number of replicas (discrete Stroom nodes) to deploy in the NodeSet
	// +kubebuilder:validation:Minimum=1
	Count int32 `json:"count"`
	// Role of each node in the NodeSet. ProcessingNodeRole nodes are limited to receiving and processing data,
	// while FrontendNodeRole nodes host the Stroom.
	//
	// If this property is omitted, nodes in the NodeSet will assume both Processing and Frontend roles.
	Role NodeRole `json:"role,omitempty"`
	// LocalDataVolumeClaim provides persistent storage for each Stroom node's data
	LocalDataVolumeClaim corev1.PersistentVolumeClaimSpec `json:"localDataVolumeClaim"`
	// Resources determine how much CPU and memory each individual Stroom node Pod within the NodeSet requests and is
	// limited to.
	Resources corev1.ResourceRequirements `json:"resources"`
	// MemoryOptions define JVM memory parameters
	MemoryOptions JvmMemoryOptions `json:"memoryOptions"`
	// IngressEnabled determines whether this node receives requests via the created Kubernetes Ingresses. Usually this
	// should be `true`, unless there is a need for a NodeSet to be pure processing-only nodes, which cannot receive data.
	// +kubebuilder:default:=true
	IngressEnabled bool `json:"ingressEnabled,omitempty"`
	// IngressAnnotations is an optional map of annotations to apply to the NodeSet's Ingress. These override any
	// default annotations provided by the controller.
	IngressAnnotations map[string]string `json:"ingressAnnotations,omitempty"`
	// ReadinessProbeTimings specify the parameters for Pod initial startup. If a node fails to start up in accordance with
	// these parameters, it will be killed. Therefore, ensure sufficient leeway is provided.
	ReadinessProbeTimings ProbeTimings `json:"readinessProbeTimings,omitempty"`
	// LivenessProbeTimings specify poll Pod healthcheck conditions. Ensure periods of high volume are factored in when
	// deciding these timings, otherwise nodes will be killed if they fail the specified parameters.
	LivenessProbeTimings ProbeTimings `json:"livenessProbeTimings,omitempty"`
	// PodAnnotations are additional annotations to set for each NodeSet Pod
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
	// PodSecurityContext applies to each NodeSet Pod
	PodSecurityContext corev1.PodSecurityContext `json:"podSecurityContext,omitempty"`
	// SecurityContext overrides the PodSecurityContext at the container level
	SecurityContext corev1.SecurityContext `json:"securityContext,omitempty"`
	// NodeSelector allows for NodeSet Pods to be deployed to a particular node, or set of nodes, by the specified labels
	NodeSelector map[string]string   `json:"nodeSelector,omitempty"`
	Tolerations  []corev1.Toleration `json:"tolerations,omitempty"`
	Affinity     corev1.Affinity     `json:"affinity,omitempty"`
}

type JvmMemoryOptions struct {
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	InitialPercentage int `json:"initialPercentage"`
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	MaxPercentage int `json:"maxPercentage"`
}

type NodeRole string

const (
	// ProcessingNodeRole applies to a NodeSet that is dedicated to receiving and processing data and does not serve
	// web front-end (UI) requests
	ProcessingNodeRole NodeRole = "Processing"

	// FrontendNodeRole applies to a NodeSet that is dedicated to serving web front-end (UI) requests only
	FrontendNodeRole = "Frontend"
)
