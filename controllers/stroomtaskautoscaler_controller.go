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
	"fmt"
	"github.com/go-logr/logr"
	controllers "github.com/p-kimberley/stroom-k8s-operator/controllers/common"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	metrics "k8s.io/metrics/pkg/apis/metrics/v1beta1"
	"math"
	"time"

	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// StroomTaskAutoscalerReconciler reconciles a StroomTaskAutoscaler object
type StroomTaskAutoscalerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger

	Metrics StroomNodeMetricMap
}

//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomtaskautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomtaskautoscalers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomtaskautoscalers/finalizers,verbs=update
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters,verbs=get;list
//+kubebuilder:rbac:groups=metrics.k8s.io,resources=pods,verbs=get;list
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *StroomTaskAutoscalerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	defaultResult := ctrl.Result{RequeueAfter: time.Second * 10}
	errorResult := ctrl.Result{}

	stroomTaskAutoscaler := stroomv1.StroomTaskAutoscaler{}
	if err := r.Get(ctx, req.NamespacedName, &stroomTaskAutoscaler); err != nil {
		if errors.IsNotFound(err) {
			return errorResult, nil
		}

		logger.Error(err, fmt.Sprintf("Unable to fetch StroomTaskAutoscaler %v", req.NamespacedName))
		return errorResult, err
	}

	stroomClusterRef := stroomTaskAutoscaler.Spec.StroomClusterRef
	if stroomClusterRef.Namespace == "" {
		// If no namespace specified, find the StroomCluster in the same namespace as the StroomTaskAutoscaler
		stroomClusterRef.Namespace = req.Namespace
	}

	// Get the StroomCluster
	stroomCluster := stroomv1.StroomCluster{}
	if err := r.Get(ctx, stroomClusterRef.NamespacedName(), &stroomCluster); err != nil {
		logger.Error(err, fmt.Sprintf("StroomCluster '%v' not found", stroomClusterRef))
		return errorResult, err
	}

	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		// Ignore dedicated UI nodes as these don't execute tasks anyway
		if nodeSet.Role == stroomv1.FrontendNodeRole {
			continue
		}

		// Get all NodeSet replica pods
		podListOptions := []client.ListOption{
			client.InNamespace(stroomCluster.Namespace),
			client.MatchingLabels(stroomCluster.GetNodeSetSelectorLabels(&nodeSet)),
		}
		nodePods := corev1.PodList{}
		if err := r.List(ctx, &nodePods, podListOptions...); err != nil {
			logger.Error(err, "Could not list NodeSet pods", "StroomCluster", stroomCluster.Name, "NodeSet", nodeSet.Name)
			return errorResult, err
		}

		// For each Pod, retrieve PodMetrics and auto-scale Stroom tasks as necessary
		for _, pod := range nodePods.Items {
			podNamespacedName := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}
			currentTime := time.Now()

			podMetrics := metrics.PodMetrics{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: pod.Namespace, Name: pod.Name}, &podMetrics); err != nil {
				logger.Info(fmt.Sprintf("PodMetrics not found for pod %v - it may be starting up", pod.Name), "Namespace", pod.Namespace)
				// Pod probably doesn't exist, so purge any metric data we have on it
				r.Metrics.DeletePodData(podNamespacedName.String())
				return defaultResult, nil
			}

			// Find the relevant container metrics
			nodeMetrics := StroomNodeMetric{}
			for _, container := range podMetrics.Containers {
				if container.Name == StroomNodeContainerName {
					nodeMetrics = StroomNodeMetric{
						Time:     podMetrics.Timestamp.Time,
						CpuUsage: container.Usage.Cpu(),
					}
					break
				}
			}

			// If this isn't a Stroom node container, ignore the metrics
			if nodeMetrics.IsZero() || nodeMetrics.CpuUsage == nil {
				continue
			}

			// Store the current metrics against the pod namespace/name
			r.Metrics.AddMetric(podNamespacedName.String(), nodeMetrics)
			r.Metrics.AgeOff(MaximumMetricRetentionPeriodMins, currentTime)

			// Determine whether the auto-scaling time interval has elapsed
			autoScaleOptions := stroomTaskAutoscaler.Spec
			adjustmentInterval := autoScaleOptions.AdjustmentIntervalMins
			if r.Metrics.ShouldScale(podNamespacedName.String(), adjustmentInterval, currentTime) {
				if err := r.scaleStroomNode(ctx, &stroomCluster, &nodeSet, podNamespacedName, &autoScaleOptions, currentTime); err != nil {
					r.Metrics.SetLastScaled(podNamespacedName.String(), currentTime)
					return errorResult, err
				} else {
					r.Metrics.SetLastScaled(podNamespacedName.String(), currentTime)
				}
			}

			// If this is the first time a pod has appeared, schedule scaling for the current time + interval
			if !r.Metrics.IsScaleScheduled(podNamespacedName.String()) {
				r.Metrics.SetLastScaled(podNamespacedName.String(), currentTime)
			}
		}
	}

	return defaultResult, nil
}

func (r *StroomTaskAutoscalerReconciler) scaleStroomNode(ctx context.Context, stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, podNamespacedName types.NamespacedName,
	autoScaleOptions *stroomv1.StroomTaskAutoscalerSpec, currentTime time.Time) error {
	logger := log.FromContext(ctx)

	// Check whether resource limits are set
	cpuLimit := nodeSet.Resources.Limits.Cpu()
	logger.Info(fmt.Sprintf("Metric count for node %v: %v", podNamespacedName.Name, len(r.Metrics.Items[podNamespacedName.String()])))
	if cpuLimit != nil {
		var avgCpuUsage int64 = 0
		if found := r.Metrics.GetSlidingWindowMean(podNamespacedName.String(), autoScaleOptions.MetricsSlidingWindowMins, currentTime, &avgCpuUsage); found {
			cpuPercent := int(math.Floor(float64(avgCpuUsage) / float64(cpuLimit.MilliValue()) * 100.0))
			logger.Info(fmt.Sprintf("CPU usage for Stroom node is on average %vm (%v percent over the past %v minutes). Limit: %vm", avgCpuUsage, cpuPercent, autoScaleOptions.MetricsSlidingWindowMins, cpuLimit.MilliValue()), "Namespace", podNamespacedName.Namespace, "Pod", podNamespacedName.Name)

			// Is CPU percentage outside bounds?
			minCpuPercent := autoScaleOptions.MinCpuPercent
			maxCpuPercent := autoScaleOptions.MaxCpuPercent
			if cpuPercent < minCpuPercent || cpuPercent > maxCpuPercent {
				// Query the node's current task limit
				dbServerRef := stroomCluster.Spec.DatabaseServerRef
				dbInfo := DatabaseConnectionInfo{}
				if err := GetDatabaseConnectionInfo(r.Client, ctx, &dbServerRef, stroomCluster.Namespace, &dbInfo); err != nil {
					return err
				}
				taskName := autoScaleOptions.TaskName
				var activeTasks, taskLimit int
				if err := r.getNodeTasks(ctx, stroomCluster, &dbInfo, podNamespacedName.Name, taskName, &activeTasks, &taskLimit); err == nil {
					newTaskLimit := taskLimit
					if cpuPercent < minCpuPercent && taskLimit < autoScaleOptions.MaxTaskLimit {
						// We're running below optimal range and have capacity to add tasks
						if activeTasks < taskLimit {
							// Node is not at capacity, so don't try and increase the task limit. This avoids scaling idle nodes.
							return nil
						}

						newTaskLimit = taskLimit + autoScaleOptions.StepAmount
						if newTaskLimit > autoScaleOptions.MaxTaskLimit {
							newTaskLimit = autoScaleOptions.MaxTaskLimit
						}
					} else if cpuPercent > maxCpuPercent && taskLimit > autoScaleOptions.MinTaskLimit {
						// We're above optimal range and can shrink the task limit
						newTaskLimit = taskLimit - autoScaleOptions.StepAmount
						if newTaskLimit < autoScaleOptions.MinTaskLimit {
							newTaskLimit = autoScaleOptions.MinTaskLimit
						}
					}
					if newTaskLimit != taskLimit {
						// Update the task limit in the DB
						logger.Info(fmt.Sprintf("Updating task limit for node '%v' from %v to %v due to CPU usage being at %v percent",
							podNamespacedName.Name, taskLimit, newTaskLimit, cpuPercent), "StroomCluster", stroomCluster.Name)
						if err := r.updateNodeTaskLimit(ctx, stroomCluster, &dbInfo, podNamespacedName.Name, taskName, newTaskLimit); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

func (r *StroomTaskAutoscalerReconciler) getNodeTasks(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, nodeName string, taskName string, activeTasks *int, taskLimit *int) error {
	logger := log.FromContext(ctx)

	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster.Namespace, stroomCluster.Spec.AppDatabaseName); err != nil {
		return err
	} else {
		defer CloseDatabase(db)

		// Get the number of active tasks and the user-defined task limit
		row := db.QueryRow(`
			select task_limit,
				(select count(*) from processor_task pt where pt.fk_processor_node_id=n.id and pt.status=?) as task_count
			from job_node jn left join job j on jn.job_id=j.id left join node n on n.name=jn.node_name
			where n.name=? and j.name=?;
			`, controllers.NodeTaskStatusProcessing, nodeName, taskName)
		if err := row.Scan(taskLimit, activeTasks); err != nil {
			logger.Error(err, "Failed to query task limit for node", "NodeName", nodeName)
			return err
		} else {
			return nil
		}
	}
}

func (r *StroomTaskAutoscalerReconciler) updateNodeTaskLimit(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, nodeName string, taskName string, taskLimit int) error {
	logger := log.FromContext(ctx)

	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster.Namespace, stroomCluster.Spec.AppDatabaseName); err != nil {
		return err
	} else {
		defer CloseDatabase(db)

		if _, err := db.Exec(`
				update job_node jn inner join job j on j.id=jn.job_id
				set task_limit=?
				where node_name=? and j.name=?;
			`, taskLimit, nodeName, taskName); err != nil {
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
		For(&stroomv1.StroomTaskAutoscaler{}).
		Complete(r)
}
