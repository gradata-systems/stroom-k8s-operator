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
