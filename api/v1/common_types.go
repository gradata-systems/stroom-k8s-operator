package v1

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
)

type ProbeTimings struct {
	// +kubebuilder:default:=5
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	// +kubebuilder:default:=5
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
	// +kubebuilder:default:=5
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
	// +kubebuilder:default:=1
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	// +kubebuilder:default:=10
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
}

// StartupProbeTimings determine how long a Stroom node is given to start up before it is restarted.
type StartupProbeTimings struct {
	// +kubebuilder:default:=5
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	// +kubebuilder:default:=5
	TimeoutSeconds int32 `json:"timeoutSeconds,omitempty"`
	// +kubebuilder:default:=5
	PeriodSeconds int32 `json:"periodSeconds,omitempty"`
	// +kubebuilder:default:=1
	SuccessThreshold int32 `json:"successThreshold,omitempty"`
	// FailureThreshold multiplied by PeriodSeconds is the base time allowed for a node to start up.
	// +kubebuilder:default:=24
	FailureThreshold int32 `json:"failureThreshold,omitempty"`
	// LockWaitAllowanceSeconds is added to the startup time allowed for each additional node in the NodeSet, when
	// the StroomCluster PodManagementPolicy is `Parallel`. This gives nodes time to wait on the Stroom bootstrap lock,
	// which is held while another node upgrades the database.
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default:=30
	LockWaitAllowanceSeconds int32 `json:"lockWaitAllowanceSeconds,omitempty"`
}

// LivenessProbeTimings determine the parameters for the Stroom node liveness probe, which queries the Stroom
// filtered health check endpoint. Allow and Deny filter which Stroom health checks are run. Each name must be a
// health check registered by Stroom, otherwise the probe will fail.
type LivenessProbeTimings struct {
	ProbeTimings `json:",inline"`
	// Allow lists the health checks to run. If empty, all health checks are run, except those listed in Deny.
	// Defaults to the JVM thread deadlock check, so nodes are not restarted when an external dependency is down.
	// Set to an empty list to run all health checks.
	// +kubebuilder:default:={"deadlocks"}
	// +kubebuilder:validation:items:Pattern=`^[^,]+$`
	Allow *[]string `json:"allow,omitempty"`
	// Deny lists the health checks to exclude.
	// +kubebuilder:validation:items:Pattern=`^[^,]+$`
	Deny *[]string `json:"deny,omitempty"`
}

// ReadinessProbeTimings determine the parameters for the Stroom node readiness probe, which queries the Stroom
// filtered health check endpoint. Allow and Deny filter which Stroom health checks are run. Each name must be a
// health check registered by Stroom, otherwise the probe will fail.
type ReadinessProbeTimings struct {
	ProbeTimings `json:",inline"`
	// Allow lists the health checks to run. If empty, all health checks are run, except those listed in Deny.
	// +kubebuilder:validation:items:Pattern=`^[^,]+$`
	Allow *[]string `json:"allow,omitempty"`
	// Deny lists the health checks to exclude. Defaults to the external identity provider check, so nodes keep
	// receiving data while the identity provider is unavailable. Set to an empty list to exclude no health checks.
	// +kubebuilder:default:={"stroom.security.common.impl.ExternalIdpConfigurationProvider"}
	// +kubebuilder:validation:items:Pattern=`^[^,]+$`
	Deny *[]string `json:"deny,omitempty"`
}

type Image struct {
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
}

func (in *Image) String() string {
	if in.Tag == "" {
		return fmt.Sprintf("%v:latest", in.Repository)
	} else {
		return fmt.Sprintf("%v:%v", in.Repository, in.Tag)
	}
}

func (in *Image) IsZero() bool {
	return in.Repository == "" && in.Tag == ""
}

type ResourceRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// String returns the general purpose string representation
func (in *ResourceRef) String() string {
	return fmt.Sprintf("%v/%v", in.Namespace, in.Name)
}

func (in *ResourceRef) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: in.Namespace,
		Name:      in.Name,
	}
}

func (in *ResourceRef) IsZero() bool {
	return in.Name == "" && in.Namespace == ""
}
