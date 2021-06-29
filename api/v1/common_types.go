package v1

import (
	"fmt"
	"k8s.io/apimachinery/pkg/types"
)

type ProbeTimings struct {
	// +kubebuilder:default:=5
	InitialDelaySeconds int32 `json:"initialDelaySeconds"`
	// +kubebuilder:default:=5
	TimeoutSeconds int32 `json:"timeoutSeconds"`
	// +kubebuilder:default:=5
	PeriodSeconds int32 `json:"periodSeconds"`
	// +kubebuilder:default:=1
	SuccessThreshold int32 `json:"successThreshold"`
	// +kubebuilder:default:=10
	FailureThreshold int32 `json:"failureThreshold"`
}

type Image struct {
	// +kubebuilder:validation:MinLength=1
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
}

func (in Image) String() string {
	return fmt.Sprintf("%v:%v", in.Repository, in.Tag)
}

type ResourceRef struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
}

// String returns the general purpose string representation
func (in ResourceRef) String() string {
	return fmt.Sprintf("%v/%v", in.Namespace, in.Name)
}

func (in ResourceRef) NamespacedName() types.NamespacedName {
	return types.NamespacedName{
		Namespace: in.Namespace,
		Name:      in.Name,
	}
}
