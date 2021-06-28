package v1

import "fmt"

type Image struct {
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

func (in Image) String() string {
	return fmt.Sprintf("%v:%v", in.Repository, in.Tag)
}

type ProbeTimings struct {
	InitialDelaySeconds int32 `json:"initialDelaySeconds,omitempty"`
	TimeoutSeconds      int32 `json:"timeoutSeconds,omitempty"`
	PeriodSeconds       int32 `json:"periodSeconds,omitempty"`
	SuccessThreshold    int32 `json:"successThreshold,omitempty"`
	FailureThreshold    int32 `json:"failureThreshold,omitempty"`
}

type ResourceRef struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
}

// String returns the general purpose string representation
func (n ResourceRef) String() string {
	return n.Namespace + "/" + n.Name
}
