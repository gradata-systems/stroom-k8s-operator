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
	"embed"
	"fmt"
	"github.com/go-logr/logr"
	controllers "github.com/p-kimberley/stroom-k8s-operator/controllers/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"path"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
)

// StroomClusterReconciler reconciles a StroomCluster object
type StroomClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

//go:embed static_content
var StaticFiles embed.FS

//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databaseservers,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databaseservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=persistentvolumeclaims,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=ingresses,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *StroomClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	stroomCluster := stroomv1.StroomCluster{}
	result := reconcile.Result{}

	if err := r.Get(ctx, req.NamespacedName, &stroomCluster); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, fmt.Sprintf("Unable to fetch StroomCluster %v", req.NamespacedName.String()))
		return ctrl.Result{}, err
	}

	// Retrieve app database connection info
	dbServerRef := stroomCluster.Spec.DatabaseServerRef
	dbInfo := DatabaseConnectionInfo{}
	if err := GetDatabaseConnectionInfo(r.Client, ctx, &stroomCluster, &dbServerRef, &dbInfo); err != nil {
		logger.Info(fmt.Sprintf("DatabaseServer '%v' could not be found", dbServerRef.ServerRef))
		return ctrl.Result{}, err
	} else if dbInfo.DatabaseServer != nil {
		if err := r.claimDatabaseServer(ctx, &stroomCluster, dbServerRef.ServerRef, dbInfo.DatabaseServer); err != nil {
			return ctrl.Result{}, err
		}
	}

	// If StroomCluster is deleted or an error occurs when adding a finalizer, do not proceed with child item creation
	var requeue, clusterDeleted bool
	if err := r.checkIfDeleted(ctx, &stroomCluster, &dbInfo, &requeue, &clusterDeleted); err != nil {
		return ctrl.Result{}, err
	} else if requeue {
		// Waiting for Stroom task completion, so check again in a minute
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if clusterDeleted {
		return ctrl.Result{}, err
	}

	// Create child objects

	foundServiceAccount := corev1.ServiceAccount{}
	result, err := r.getOrCreateObject(ctx, stroomCluster.GetBaseName(), stroomCluster.Namespace, "ServiceAccount", &foundServiceAccount, func() error {
		// Create a new ServiceAccount
		resource := r.createServiceAccount(&stroomCluster)
		logger.Info("Creating a new ServiceAccount", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Get the API key of the Stroom internal processing user, so it can be used to query the API in lifecycle scripts
	apiKey := ""
	if err := r.getApiKey(ctx, &stroomCluster, &dbInfo, &apiKey); err != nil {
		return ctrl.Result{}, err
	}

	// Create a Secret containing the Stroom API key
	foundSecret := corev1.Secret{}
	result, err = r.getOrCreateObject(ctx, stroomCluster.GetBaseName(), stroomCluster.Namespace, "Secret", &foundSecret, func() error {
		// Create the Secret
		resource := r.createSecret(&stroomCluster, apiKey)
		logger.Info("Creating new Secret", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Create a ConfigMap containing Stroom configuration and lifecycle scripts
	foundConfigMap := corev1.ConfigMap{}
	result, err = r.getOrCreateObject(ctx, stroomCluster.GetBaseName(), stroomCluster.Namespace, "ConfigMap", &foundConfigMap, func() error {
		if files, err := StaticFiles.ReadDir("static_content"); err != nil {
			logger.Error(err, "Could not read static files to populate ConfigMap", "StroomCluster", stroomCluster.Name)
			return err
		} else {
			allFileData := make(map[string]string)
			for _, file := range files {
				if data, err := StaticFiles.ReadFile(path.Join("static_content", file.Name())); err != nil {
					logger.Error(err, "Could not read static file", "Filename", file.Name())
					return err
				} else {
					allFileData[file.Name()] = string(data)
				}
			}

			// Create the ConfigMap
			resource := r.createConfigMap(&stroomCluster, allFileData)
			logger.Info("Creating StroomCluster ConfigMap", "Namespace", resource.Namespace, "Name", resource.Name)
			return r.Create(ctx, resource)
		}
	})
	if err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Query the StroomCluster StatefulSet and if it doesn't exist, create it
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		foundStatefulSet := appsv1.StatefulSet{}
		result, err = r.getOrCreateObject(ctx, stroomCluster.GetNodeSetName(nodeSet.Name), stroomCluster.Namespace, "StatefulSet", &foundStatefulSet, func() error {
			// Create a StatefulSet for the NodeSet
			resource := r.createStatefulSet(&stroomCluster, &nodeSet, &dbInfo)
			logger.Info("Creating a new StatefulSet", "Namespace", resource.Namespace, "Name", resource.Name)
			return r.Create(ctx, resource)
		})
		if err != nil {
			return result, err
		} else if !result.IsZero() {
			// StatefulSet was created (didn't exist before), so requeue
			return result, nil
		}

		// StatefulSet already exists, so update it based on any StroomCluster/NodeSet configuration changes
		if err := r.updateNodeSet(ctx, &stroomCluster, &nodeSet, &foundStatefulSet); err != nil {
			return ctrl.Result{}, err
		}

		foundService := corev1.Service{}
		result, err = r.getOrCreateObject(ctx, stroomCluster.GetNodeSetServiceName(nodeSet.Name), stroomCluster.Namespace, "Service", &foundService, func() error {
			// Create a headless service for the NodeSet
			resource := r.createService(&stroomCluster, &nodeSet)
			logger.Info("Creating a new Service", "Namespace", resource.Namespace, "Name", resource.Name)
			return r.Create(ctx, resource)
		})
		if err != nil {
			return result, err
		} else if !result.IsZero() {
			return result, nil
		}
	}

	ingresses := r.createIngresses(ctx, &stroomCluster)
	for _, ingress := range ingresses {
		// Create an Ingress if it doesn't already exist
		foundIngress := v1.Ingress{}
		result, err = r.getOrCreateObject(ctx, ingress.Name, ingress.Namespace, "Ingress", &foundIngress, func() error {
			// Create an Ingress
			logger.Info("Creating a new Ingress", "Namespace", ingress.Namespace, "Name", ingress.Name)
			if err := r.Create(ctx, &ingress); err != nil {
				return err
			}
			return nil
		})
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	// TODO: Add node list to status

	return ctrl.Result{}, nil
}

func (r *StroomClusterReconciler) updateNodeSet(ctx context.Context, stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, statefulSet *appsv1.StatefulSet) error {
	logger := log.FromContext(ctx)
	podSpec := &statefulSet.Spec.Template.Spec

	// Update the NodeSet replica count if different to the spec
	oldReplicaCount := *statefulSet.Spec.Replicas
	newReplicaCount := nodeSet.Count
	if oldReplicaCount != newReplicaCount {
		// Scale the NodeSet
		statefulSet.Spec.Replicas = &nodeSet.Count
		logger.Info(fmt.Sprintf("NodeSet replicas changed from %v to %v", oldReplicaCount, newReplicaCount),
			"StroomCluster", stroomCluster.Name, "NodeSet", nodeSet.Name)

		// Delete excess PVCs, depending on deletion policy
		if err := r.deletePvcs(ctx, stroomCluster, nodeSet, oldReplicaCount, newReplicaCount); err != nil {
			return err
		}
	}

	// Update container image and/or tag if different
	oldImage := podSpec.Containers[0].Image
	newImage := stroomCluster.Spec.Image
	if newImage.String() != oldImage {
		podSpec.Containers[0].Image = newImage.String()
		logger.Info(fmt.Sprintf("Stroom node pod image changed to '%v'", newImage.String()), "StroomCluster", stroomCluster.Name)
	}

	// Check other properties

	if podSpec.Containers[0].ImagePullPolicy != stroomCluster.Spec.ImagePullPolicy {
		podSpec.Containers[0].ImagePullPolicy = stroomCluster.Spec.ImagePullPolicy
		logger.Info("ImagePullPolicy changed", "StroomCluster", stroomCluster.Name)
	}
	if *podSpec.TerminationGracePeriodSeconds != stroomCluster.Spec.NodeTerminationPeriodSecs {
		*podSpec.TerminationGracePeriodSeconds = stroomCluster.Spec.NodeTerminationPeriodSecs
		logger.Info("TerminationGracePeriodSeconds changed", "StroomCluster", stroomCluster.Name)
	}

	// Commit the update
	if err := r.Update(ctx, statefulSet); err != nil {
		logger.Error(err, "NodeSet StatefulSet could not be updated", "StroomCluster", stroomCluster.Name, "NodeSet", nodeSet.Name)
		return err
	} else {
		logger.Info("NodeSet configuration updated", "StroomCluster", stroomCluster.Name, "NodeSet", nodeSet.Name)
	}

	return nil
}

func (r *StroomClusterReconciler) deletePvcs(ctx context.Context, stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, oldReplicaCount int32, newReplicaCount int32) error {
	logger := log.FromContext(ctx)

	// Depending on the VolumeClaimDeletePolicy, delete any PVCs no longer associated with pods
	pvcDeletePolicy := stroomCluster.Spec.VolumeClaimDeletePolicy
	if oldReplicaCount > newReplicaCount && (pvcDeletePolicy == stroomv1.DeleteOnScaledownAndClusterDeletionPolicy || pvcDeletePolicy == stroomv1.DeleteOnScaledownOnlyPolicy) {
		// NodeSet has scaled down, so remove any PVCs associated with deleted pods
		for podOrdinal := newReplicaCount + 1; podOrdinal <= oldReplicaCount; podOrdinal++ {
			// Attempt to delete PVC named in accordance with convention:
			// <PVC name>-<NodeSet name>-<ordinal>
			pvcName := fmt.Sprintf("%v-%v-%v", StroomNodePvcName, stroomCluster.GetNodeSetName(nodeSet.Name), podOrdinal-1)
			foundPvc := corev1.PersistentVolumeClaim{}
			if err := r.Get(ctx, types.NamespacedName{Namespace: stroomCluster.Namespace, Name: pvcName}, &foundPvc); err != nil {
				logger.Error(err, "Could not find PVC in order to delete it", "Namespace", stroomCluster.Namespace, "Name", pvcName)
				// Continue, as this isn't a critical error
			} else {
				if err := r.Delete(ctx, &foundPvc); err != nil {
					logger.Error(err, "PVC could not be deleted", "Namespace", foundPvc.Namespace, "Name", foundPvc.Name)
				} else {
					logger.Info("PVC deleted due to StroomCluster deletion policy", "Namespace", foundPvc.Namespace, "Name", foundPvc.Name)
				}
			}
		}
	}

	return nil
}

// checkIfDeleted returns whether the object is being deleted and if any error occurred during finalisation
func (r *StroomClusterReconciler) checkIfDeleted(ctx context.Context, stroomCluster *stroomv1.StroomCluster, appDatabase *DatabaseConnectionInfo,
	requeue *bool, clusterDeleted *bool) error {
	logger := log.FromContext(ctx)

	*clusterDeleted = false
	*requeue = false

	if !stroomCluster.IsBeingDeleted() {
		// Add finalizers to database server objects to prevent them from being removed while the StroomCluster
		// still exists
		if err := r.addFinalizer(ctx, appDatabase.DatabaseServer, stroomv1.StroomClusterFinalizerName); err != nil {
			return err
		}

		// Add a finalizer to the StroomCluster to wait for Stroom node tasks to drain
		if err := r.addFinalizer(ctx, stroomCluster, stroomv1.WaitNodeTasksFinalizerName); err != nil {
			return err
		}
	} else {
		// Cluster is being deleted
		*clusterDeleted = true

		// Disable node task processing, allowing nodes to drain
		if err := r.disableTaskProcessing(ctx, stroomCluster, appDatabase); err != nil {
			return err
		} else {
			logger.Info("Task processing disabled, nodes draining", "StroomCluster", stroomCluster.Name)
		}

		// Check whether there are any active Stroom node tasks. Only allow deletion once they are completed.
		remainingTasks := make(map[string]int)
		if err := r.countRemainingTasks(ctx, stroomCluster, appDatabase, remainingTasks); err != nil {
			*requeue = true
			return err
		} else if len(remainingTasks) > 0 {
			// Requeue so we can check again after some time, to allow node server tasks to finish
			remainingTaskSummary := fmt.Sprintf("StroomCluster deletion waiting on task completion for %v nodes: ", len(remainingTasks))
			for nodeName, taskCount := range remainingTasks {
				remainingTaskSummary += fmt.Sprintf("%v (%v) ", nodeName, taskCount)
			}
			logger.Info(remainingTaskSummary, "StroomCluster", stroomCluster.Name)
			*requeue = true
			return nil
		} else {
			// All tasks drained, so allow deletion by removing the finalizer
			logger.Info("All tasks drained, deletion commencing", "StroomCluster", stroomCluster.Name)
			if err := r.removeFinalizer(ctx, stroomCluster, stroomv1.WaitNodeTasksFinalizerName); err != nil {
				return err
			}
		}

		// Remove finalizer from the linked DatabaseServers
		if err := r.removeFinalizer(ctx, appDatabase.DatabaseServer, stroomv1.StroomClusterFinalizerName); err != nil {
			logger.Error(err, "Finalizer could not be removed from DatabaseServer",
				"Namespace", appDatabase.DatabaseServer.Namespace, "Name", appDatabase.DatabaseServer.Name)
			return err
		}

		logger.Info("StroomCluster deleted", "Namespace", stroomCluster.Namespace, "Name", stroomCluster.Name)

		r.cleanup(ctx, stroomCluster)
		return nil
	}

	return nil
}

func (r *StroomClusterReconciler) addFinalizer(ctx context.Context, obj client.Object, finalizerName string) error {
	if obj != nil {
		if !controllerutil.ContainsFinalizer(obj, finalizerName) {
			// Finalizer hasn't been added, so add it to prevent the DatabaseServer from being deleted while the dependent StroomCluster still exists
			controllerutil.AddFinalizer(obj, finalizerName)
			return r.Update(ctx, obj)
		}
	}

	return nil
}

func (r *StroomClusterReconciler) removeFinalizer(ctx context.Context, obj client.Object, finalizerName string) error {
	if obj != nil {
		if err := r.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, obj); err != nil {
			return err
		} else if controllerutil.ContainsFinalizer(obj, finalizerName) {
			controllerutil.RemoveFinalizer(obj, finalizerName)
			return r.Update(ctx, obj)
		}
	}

	return nil
}

// disableTaskProcessing disables Stroom nodes tasks prior to deleting the StroomCluster. This allows all nodes to drain.
func (r *StroomClusterReconciler) disableTaskProcessing(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo) error {
	logger := log.FromContext(ctx)

	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster); err != nil {
		return err
	} else {
		defer CloseDatabase(db)

		if _, err := db.Exec("update job_node set enabled = 0 where node_name like ?", stroomCluster.GetBaseName()+"%"); err != nil {
			logger.Error(err, "Failed to disable Stroom node task processing", "StroomCluster", stroomCluster.Name)
			return err
		}
	}

	return nil
}

// countRemainingTasks removes the finalizer from StroomCluster pods that have no active Stroom tasks.
// Returns whether tasks are still running on any node.
func (r *StroomClusterReconciler) countRemainingTasks(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, remainingTasks map[string]int) error {
	logger := log.FromContext(ctx)

	// Get the current active server tasks
	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster); err != nil {
		return err
	} else {
		defer CloseDatabase(db)

		// Get a summary of active tasks by node
		rows, err := db.Query("select n.name as node_name, count(*) as task_count "+
			"from processor_task pt inner join node n on n.id = pt.fk_processor_node_id "+
			"where pt.status = ? "+
			"group by n.name", controllers.NodeTaskStatusProcessing)

		if err != nil {
			logger.Error(err, fmt.Sprintf("Failed to query the active Stroom processor tasks for cluster '%v'", stroomCluster.Name))
			return err
		}

		// For each pod in the StroomCluster determine whether any active Stroom tasks are running
		var nodeName string
		var taskCount int
		for rows.Next() {
			if err := rows.Scan(&nodeName, &taskCount); err != nil {
				logger.Error(err, fmt.Sprintf("Could not parse node name and task count"))
				return err
			} else if taskCount > 0 {
				remainingTasks[nodeName] = taskCount
			}
		}
	}

	return nil
}

// cleanup performs post-deletion actions like removing Ingress resources created by the operator
func (r *StroomClusterReconciler) cleanup(ctx context.Context, stroomCluster *stroomv1.StroomCluster) {
	logger := log.FromContext(ctx)

	// Both Ingress and PVC objects share the same labels and namespace
	listOptions := []client.ListOption{
		client.InNamespace(stroomCluster.Namespace),
		client.MatchingLabels(stroomCluster.GetLabels()),
	}

	// Remove any Ingress objects created by the operator
	var ingressList v1.IngressList
	if err := r.List(ctx, &ingressList, listOptions...); err != nil {
		logger.Error(err, "Failed to list Ingresses by label", "ClusterName", stroomCluster.Name)
	} else {
		for _, ingress := range ingressList.Items {
			if err := r.Delete(ctx, &ingress); err != nil {
				logger.Error(err, "Failed to delete Ingress", "IngressName", ingress.Name)
			}
		}
	}

	// Delete PVCs in accordance with the VolumeClaimDeletePolicy
	if stroomCluster.Spec.VolumeClaimDeletePolicy == stroomv1.DeleteOnScaledownAndClusterDeletionPolicy {
		// Cluster is being deleted, so remove the NodeSet PVCs
		var pvcList corev1.PersistentVolumeClaimList
		if err := r.List(ctx, &pvcList, listOptions...); err != nil {
			logger.Error(err, "Failed to list PVCs by label", "ClusterName", stroomCluster.Name)
		} else {
			for _, pvc := range pvcList.Items {
				if err := r.Delete(ctx, &pvc); err != nil {
					logger.Error(err, "Failed to delete PVC", "IngressName", pvc.Name)
				}
			}
		}
	}
}

func (r *StroomClusterReconciler) getOrCreateObject(ctx context.Context, name string, namespace string, objectType string, foundObject client.Object, onCreate func() error) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundObject); err != nil && errors.IsNotFound(err) {
		// Attempt to create the object, as it doesn't exist
		err = onCreate()

		if err != nil {
			logger.Error(err, "Failed to create new object", "Type", objectType, "Namespace", namespace, "Name", name)
			return ctrl.Result{}, err
		}

		// Object created successfully, so return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get object", "Type", objectType)
		return ctrl.Result{}, err
	}

	// Object exists and was successfully retrieved
	return ctrl.Result{}, nil
}

func (r *StroomClusterReconciler) claimDatabaseServer(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbRef stroomv1.ResourceRef, db *stroomv1.DatabaseServer) error {
	logger := log.FromContext(ctx)

	// If DatabaseServer is claimed by a StroomCluster, check whether it is the current cluster
	if !db.StroomClusterRef.IsZero() {
		if db.StroomClusterRef.Name == stroomCluster.Name && db.StroomClusterRef.Namespace == stroomCluster.Namespace {
			// Already claimed by this cluster
			return nil
		}

		// Already owned by another cluster, so we can't claim it
		err := errors.NewBadRequest(fmt.Sprintf("DatabaseServer '%v/%v' already claimed by StroomCluster '%v'. Cannot be claimed by StroomCluster '%v/%v'",
			db.Namespace, db.Name, db.StroomClusterRef, stroomCluster.Namespace, stroomCluster.Name))
		logger.Error(err, "Cannot claim DatabaseServer")
		return err
	} else {
		// Register the StroomCluster with the DatabaseServer
		db.StroomClusterRef = dbRef
		err := r.Update(ctx, db)
		if err != nil {
			logger.Error(err, fmt.Sprintf("Could not claim the DatabaseServer '%v' by StroomCluster '%v/%v'", dbRef, stroomCluster.Namespace, stroomCluster.Name))
			return err
		}
	}

	return nil
}

// getApiKey retrieves the first active API key created by the Stroom internal processing user
func (r *StroomClusterReconciler) getApiKey(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbInfo *DatabaseConnectionInfo, apiKey *string) error {
	logger := log.FromContext(ctx)

	if db, err := OpenDatabase(r, ctx, dbInfo, stroomCluster); err != nil {
		return err
	} else {
		defer CloseDatabase(db)

		row := db.QueryRow("select data from token where create_user = ? and enabled = 1 limit 1", stroomv1.StroomInternalUserName)
		if err := row.Scan(apiKey); err != nil {
			logger.Error(err, "Could not retrieve API key from database", "User", stroomv1.StroomInternalUserName)
			return err
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StroomClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stroomv1.StroomCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}
