/*
Copyright 2021.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// StroomTaskAutoscalerSpec defines the desired state of StroomTaskAutoscaler
type StroomTaskAutoscalerSpec struct {
	// The target StroomCluster to apply autoscaling to
	StroomClusterRef ResourceRef `json:"stroomClusterRef"`

	// Name of the Stroom node task to auto-scale. Usually "Data Processor".
	// +kubebuilder:validation:Required
	TaskName string `json:"taskName"`

	// How often (in minutes) adjustments are made to the number of Stroom node tasks
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum:=1
	AdjustmentIntervalMins int `json:"adjustmentIntervalMins,omitempty"`

	// Sliding window (in minutes) over which to calculate CPU usage vs. the threshold parameters
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum:=1
	MetricsSlidingWindowMins int `json:"metricsSlidingWindowMins,omitempty"`

	// Minimum CPU usage threshold before the number of tasks is adjust upwards
	// +kubebuilder:default:=50
	MinCpuPercent int `json:"minCpuPercent,omitempty"`

	// Maximum CPU usage threshold before the number of tasks is adjusted downwards
	// +kubebuilder:default:=90
	MaxCpuPercent int `json:"maxCpuPercent,omitempty"`

	// Minimum number of tasks auto-scaler may set the node limit to
	// +kubebuilder:default:=1
	MinTaskLimit int `json:"minTaskLimit,omitempty"`

	// Maximum number of tasks auto-scaler may set the node limit to
	// +kubebuilder:default:=20
	MaxTaskLimit int `json:"maxTaskLimit,omitempty"`

	// Number of tasks to add/subtract each adjustment interval, based on usage
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum:=1
	StepAmount int `json:"stepAmount,omitempty"`
}

// StroomTaskAutoscalerStatus defines the observed state of StroomTaskAutoscaler
type StroomTaskAutoscalerStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// StroomTaskAutoscaler is the Schema for the stroomtaskautoscalers API
type StroomTaskAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   StroomTaskAutoscalerSpec   `json:"spec,omitempty"`
	Status StroomTaskAutoscalerStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// StroomTaskAutoscalerList contains a list of StroomTaskAutoscaler
type StroomTaskAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []StroomTaskAutoscaler `json:"items"`
}

func init() {
	SchemeBuilder.Register(&StroomTaskAutoscaler{}, &StroomTaskAutoscalerList{})
}
