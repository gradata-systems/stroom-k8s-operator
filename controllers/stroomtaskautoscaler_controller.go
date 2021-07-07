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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"github.com/go-logr/logr"
	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	metrics "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"math"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"time"
)

// StroomTaskAutoscalerReconciler reconciles a StroomTaskAutoscaler object
type StroomTaskAutoscalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger

	Metrics NodeMetricMap
}

//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomtaskautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomtaskautoscalers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomtaskautoscalers/finalizers,verbs=update
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters,verbs=get;list;update
//+kubebuilder:rbac:groups=metrics.k8s.io,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *StroomTaskAutoscalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	podMetrics := metrics.PodMetrics{}
	if err := r.Get(ctx, req.NamespacedName, &podMetrics); err != nil {
		logger.Error(err, "Could not retrieve PodMetrics", "Namespace", req.Namespace, "Name", req.Name)
		return ctrl.Result{}, err
	}

	nodeMetrics := NodeMetric{}
	for _, container := range podMetrics.Containers {
		if container.Name == StroomNodeContainerName {
			nodeMetrics = NodeMetric{
				Time:     podMetrics.Timestamp.Time,
				CpuUsage: container.Usage.Cpu(),
			}
			break
		}
	}

	// If this isn't a Stroom node container, ignore the metrics
	if nodeMetrics.IsZero() || nodeMetrics.CpuUsage == nil {
		return ctrl.Result{}, nil
	}

	podNamespacedName := req.NamespacedName.String()
	currentTime := time.Now()

	// Store the current metrics against the pod namespace/name
	r.Metrics.AddMetric(podNamespacedName, nodeMetrics)
	r.Metrics.AgeOff(MaximumMetricRetentionPeriodMins, currentTime)

	nodePod := corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, &nodePod); err != nil {
		logger.Error(err, "Could not retrieve Pod", "Namespace", req.Namespace, "Name", req.Name)
		return ctrl.Result{}, err
	}

	clusterName, clusterNameExists := nodePod.GetLabels()[stroomv1.StroomClusterLabel]
	nodeSetName, nodeSetNameExists := nodePod.GetLabels()[stroomv1.NodeSetLabel]
	if !clusterNameExists || !nodeSetNameExists {
		// Not a Stroom node pod
		return ctrl.Result{}, nil
	}

	// Get the StroomCluster using the namespace and cluster name from the Pod label
	stroomCluster := stroomv1.StroomCluster{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: clusterName}, &stroomCluster); err != nil {
		logger.Error(err, fmt.Sprintf("StroomCluster '%v' not found", clusterName), "Namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	// Find the NodeSet in the StroomCluster using the Pod label
	var nodeSet *stroomv1.NodeSet
	for _, ns := range stroomCluster.Spec.NodeSets {
		if ns.Name == nodeSetName {
			nodeSet = &ns
			break
		}
	}
	if nodeSet == nil {
		err := errors.New("NodeSet not found")
		logger.Error(err, "Could not find NodeSet", "StroomCluster", stroomCluster.Name, "Namespace", req.Namespace)
		return ctrl.Result{}, err
	}

	// Determine whether the auto-scaling time interval has elapsed
	autoScaleOptions := nodeSet.TaskAutoScaleOptions
	adjustmentInterval := autoScaleOptions.AdjustmentIntervalMins
	if r.Metrics.ShouldScale(podNamespacedName, adjustmentInterval, currentTime) {
		// Ensure we update the last scaled time once finished
		defer func() {
			r.Metrics.SetLastScaled(podNamespacedName, currentTime)
		}()

		// Check whether resource limits are set
		cpuLimit := nodeSet.Resources.Limits.Cpu()
		if cpuLimit != nil {
			var avgCpuUsage int64 = 0
			if found := r.Metrics.GetSlidingWindowMean(podNamespacedName, autoScaleOptions.MetricsSlidingWindowMins, currentTime, &avgCpuUsage); found {
				cpuPercent := int(math.Floor(float64(avgCpuUsage/cpuLimit.MilliValue()) * 100.0))
				logger.Info(fmt.Sprintf("CPU usage for Stroom node is %v percent", cpuPercent), "Namespace", req.Namespace, "Pod", req.Name)

				// Is CPU percentage outside bounds?
				minPercent := autoScaleOptions.MinCpuPercent
				maxPercent := autoScaleOptions.MaxCpuPercent
				if cpuPercent < minPercent || cpuPercent > maxPercent {
					// Query the node's current task limit
					taskLimit := 0
					dbServerRef := stroomCluster.Spec.DatabaseServerRef
					dbInfo := DatabaseConnectionInfo{}
					if err := GetDatabaseConnectionInfo(r.Client, ctx, &stroomCluster, &dbServerRef, &dbInfo); err != nil {
						return ctrl.Result{}, err
					}
					if err := r.getNodeTaskLimit(ctx, &stroomCluster, &dbInfo, req.Name, &taskLimit); err == nil {
						newTaskLimit := taskLimit
						if cpuPercent < minPercent && taskLimit < autoScaleOptions.MaxTaskLimit {
							// We're running below optimal range and have capacity to add tasks
							newTaskLimit = taskLimit + autoScaleOptions.StepAmount
							if newTaskLimit > autoScaleOptions.MaxTaskLimit {
								newTaskLimit = autoScaleOptions.MaxTaskLimit
							}
						} else if cpuPercent > maxPercent && taskLimit > autoScaleOptions.MinTaskLimit {
							// We're above optimal range and can shrink the task limit
							newTaskLimit = taskLimit - autoScaleOptions.StepAmount
							if newTaskLimit < autoScaleOptions.MinTaskLimit {
								newTaskLimit = autoScaleOptions.MinTaskLimit
							}
						}
						if newTaskLimit != taskLimit {
							// Update the task limit in the DB
							logger.Info(fmt.Sprintf("Updating task limit for node '%v' from %v to %v due to CPU usage being at %v percent",
								req.Name, taskLimit, newTaskLimit, cpuPercent), "StroomCluster", stroomCluster.Name)
							if err := r.updateNodeTaskLimit(ctx, &stroomCluster, &dbInfo, req.Name, newTaskLimit); err != nil {
								return ctrl.Result{}, err
							}
						}
					}
				}
			}
		}
	}

	r.Metrics.SetLastScaled(podNamespacedName, currentTime)

	return ctrl.Result{}, nil
}

func (r *StroomTaskAutoscalerReconciler) getNodeTaskLimit(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, nodeName string, taskLimit *int) error {
	logger := log.FromContext(ctx)

	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster); err != nil {
		return err
	} else {
		row := db.QueryRow("select task_limit from job_node where node_name = ?", nodeName)
		if err := row.Scan(&taskLimit); err != nil {
			logger.Error(err, "Failed to query task limit for node", "NodeName", nodeName)
			return err
		} else {
			return nil
		}
	}
}

func (r *StroomTaskAutoscalerReconciler) updateNodeTaskLimit(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, nodeName string, taskLimit int) error {
	logger := log.FromContext(ctx)

	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster); err != nil {
		return err
	} else {
		if _, err := db.Exec("update job_node set task_limit = ? where node_name = ?", taskLimit, nodeName); err != nil {
			logger.Error(err, "Failed to update task limit for node", "NodeName", nodeName, "TaskLimit", taskLimit)
			return err
		} else {
			logger.Info("Updated task limit for node", "NodeName", nodeName, "TaskLimit", taskLimit)
			return nil
		}
	}
}

func (r *StroomTaskAutoscalerReconciler) autoScaleStroomNodeTasks(ctx context.Context, stroomCluster *stroomv1.StroomCluster) error {
	logger := log.FromContext(ctx)

	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		// Get StroomCluster pods
		podListOptions := []client.ListOption{
			client.InNamespace(stroomCluster.Namespace),
			client.MatchingLabels(stroomCluster.GetNodeSetSelectorLabels(&nodeSet)),
		}
		podList := corev1.PodList{}
		if err := r.List(ctx, &podList, podListOptions...); err != nil {
			logger.Error(err, "Failed to list Pods", "StroomCluster", stroomCluster.Name, "NodeSet", nodeSet.Name)
			return err
		}

		// Retrieve pod metrics for the NodeSet Pod
		for _, pod := range podList.Items {
			podMetrics := metrics.PodMetrics{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: stroomCluster.Namespace, Name: pod.Name}, &podMetrics); err != nil {
				logger.Error(err, "PodMetrics not found", "Namespace", stroomCluster.Namespace, "Name", pod.Name)
				return err
			} else {
				logger.Info(fmt.Sprintf("CPU usage for %v: %v", pod.Name, podMetrics.Containers[0].Usage.Cpu()))
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StroomTaskAutoscalerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Metrics = NewNodeMetricMap()

	return ctrl.NewControllerManagedBy(mgr).
		For(&metrics.PodMetrics{}).
		//Watches(&source.Kind{Type: &v1beta1.PodMetrics{}}, &handler.EnqueueRequestForObject{}).
		Complete(r)
}
