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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	v1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

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

//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databaseservers,verbs=get;list;watch;update
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
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

	// If item is deleted or an error occurs when adding a finalizer, bail out
	if deleted, err := r.checkIfDeleted(ctx, &stroomCluster); deleted || err != nil {
		return ctrl.Result{}, err
	}

	// Retrieve app database connection info
	appDatabaseRef := stroomCluster.Spec.AppDatabaseRef
	appDatabaseConnectionInfo := DatabaseConnectionInfo{}
	if result, err := r.getDatabaseConnectionInfo(ctx, &stroomCluster, &appDatabaseRef, &appDatabaseConnectionInfo); err != nil {
		logger.Info(fmt.Sprintf("DatabaseServer '%v' could not be found", appDatabaseRef.DatabaseServerRef))
		return ctrl.Result{}, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Retrieve stats database connection info
	statsDatabaseRef := stroomCluster.Spec.StatsDatabaseRef
	statsDatabaseConnectionInfo := DatabaseConnectionInfo{}
	if result, err := r.getDatabaseConnectionInfo(ctx, &stroomCluster, &statsDatabaseRef, &statsDatabaseConnectionInfo); err != nil {
		return ctrl.Result{}, err
	} else if !result.IsZero() {
		return result, nil
	}

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

	// Check the StroomCluster ConfigMap exists
	foundConfigMap := corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: stroomCluster.Spec.ConfigMapName, Namespace: stroomCluster.Namespace}, &foundConfigMap)
	if err != nil {
		logger.Error(err, fmt.Sprintf("ConfigMap '%v' referenced by StroomCluster '%v' was not found", stroomCluster.Spec.ConfigMapName, stroomCluster.Name))
		return ctrl.Result{}, err
	} else if !result.IsZero() {
		return result, nil
	}

	// Query the StroomCluster StatefulSet and if it doesn't exist, create it
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		foundStatefulSet := appsv1.StatefulSet{}
		result, err = r.getOrCreateObject(ctx, stroomCluster.GetNodeSetName(nodeSet.Name), stroomCluster.Namespace, "StatefulSet", &foundStatefulSet, func() error {
			// Create a StatefulSet for the NodeSet
			resource := r.createStatefulSet(&stroomCluster, &nodeSet, &appDatabaseConnectionInfo, &statsDatabaseConnectionInfo)
			logger.Info("Creating a new StatefulSet", "Namespace", resource.Namespace, "Name", resource.Name)
			return r.Create(ctx, resource)
		})
		if err != nil {
			return result, err
		} else if !result.IsZero() {
			return result, nil
		}

		// Update the NodeSet replica count if different to the spec
		currentReplicaCount := foundStatefulSet.Spec.Replicas
		newReplicaCount := nodeSet.Count
		if currentReplicaCount != &newReplicaCount {
			foundStatefulSet.Spec.Replicas = &nodeSet.Count
			if err := r.Update(ctx, &foundStatefulSet); err != nil {
				logger.Error(err, fmt.Sprintf("NodeSet replica count could not be scaled from %v to %v", currentReplicaCount, newReplicaCount))
				return ctrl.Result{}, err
			}
		}

		// Disable all server tasks for nodes within the NodeSet
		if nodeSet.Role == stroomv1.Frontend {
			if err := r.enableNodeSetServerTasks(ctx, &appDatabaseConnectionInfo, &stroomCluster, &nodeSet, false); err != nil {
				return ctrl.Result{}, err
			}
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

func (r *StroomClusterReconciler) checkIfDeleted(ctx context.Context, stroomCluster *stroomv1.StroomCluster) (bool, error) {
	logger := log.FromContext(ctx)

	if !stroomCluster.IsBeingDeleted() {
		if !controllerutil.ContainsFinalizer(stroomCluster, stroomv1.StroomClusterFinalizerName) {
			// Finalizer hasn't been added, so add it to prevent the DatabaseServer from being deleted while the dependent StroomCluster still exists
			controllerutil.AddFinalizer(stroomCluster, stroomv1.StroomClusterFinalizerName)
			if err := r.Update(ctx, stroomCluster); err != nil {
				return false, err
			}
		}
	} else {
		if controllerutil.ContainsFinalizer(stroomCluster, stroomv1.StroomClusterFinalizerName) {
			// TODO: Add deletion blocking logic

			// Remove finalizer from the linked DatabaseServers
			appDbResult := r.removeDatabaseFinalizer(ctx, stroomCluster, stroomCluster.Spec.AppDatabaseRef.DatabaseServerRef)
			statsDbResult := r.removeDatabaseFinalizer(ctx, stroomCluster, stroomCluster.Spec.StatsDatabaseRef.DatabaseServerRef)
			if appDbResult != nil && statsDbResult != nil {
				// A failure occurred, so block deletion until this resolves
				return true, nil
			}

			// Remove the finalizer, allowing the StroomCluster to be removed
			controllerutil.RemoveFinalizer(stroomCluster, stroomv1.StroomClusterFinalizerName)
			if err := r.Update(ctx, stroomCluster); err != nil {
				logger.Error(err, fmt.Sprintf("Finalizer could not be removed from StroomCluster '%v/%v'", stroomCluster.Namespace, stroomCluster.Name))
				return true, err
			}

			logger.Info(fmt.Sprintf("StroomCluster '%v/%v' deleted", stroomCluster.Namespace, stroomCluster.Name))
		}

		r.cleanup(ctx, stroomCluster)
		return true, nil
	}

	return false, nil
}

// cleanup performs post-deletion actions like removing Ingress resources created by the operator
func (r *StroomClusterReconciler) cleanup(ctx context.Context, stroomCluster *stroomv1.StroomCluster) {
	logger := log.FromContext(ctx)

	// Remove any Ingress objects created by the operator
	ingressNames := []string{
		stroomCluster.GetBaseName(),
		stroomCluster.GetBaseName() + "-clustercall",
		stroomCluster.GetBaseName() + "-datafeed",
	}
	for _, ingressName := range ingressNames {
		ingressRef := types.NamespacedName{Namespace: stroomCluster.Namespace, Name: ingressName}
		ingress := v1.Ingress{}
		if err := r.Get(ctx, ingressRef, &ingress); err != nil {
			logger.Error(err, fmt.Sprintf("Could not fetch Ingress '%v' for deletion", ingressRef))
		} else {
			if err := r.Delete(ctx, &ingress); err != nil {
				logger.Error(err, fmt.Sprintf("Could not delete Ingress '%v'", err))
			}
		}
	}
}

func (r *StroomClusterReconciler) removeDatabaseFinalizer(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbRef stroomv1.ResourceRef) error {
	logger := log.FromContext(ctx)
	dbServer := stroomv1.DatabaseServer{}

	if dbRef == (stroomv1.ResourceRef{}) {
		return nil
	}

	// Use the StroomCluster namespace if none specified
	if dbRef.Namespace == "" {
		dbRef.Namespace = stroomCluster.Namespace
	}

	if err := r.Get(ctx, dbRef.NamespacedName(), &dbServer); err == nil {
		controllerutil.RemoveFinalizer(&dbServer, stroomv1.StroomClusterFinalizerName)
		if err := r.Update(ctx, &dbServer); err == nil {
			return nil
		} else {
			logger.Error(err, fmt.Sprintf("Could not remove finalizer from DatabaseServer '%v'", dbRef))
			return err
		}
	} else {
		logger.Error(err, fmt.Sprintf("Could not find DatabaseServer '%v' in order to remove finalizer", dbRef))
		// Return `nil` to allow deletion of StroomCluster to continue. DatabaseServer may have been deleted manually.
		return nil
	}
}

func (r *StroomClusterReconciler) getOrCreateObject(ctx context.Context, name string, namespace string, objectType string, foundObject client.Object, onCreate func() error) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	if err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundObject); err != nil && errors.IsNotFound(err) {
		// Attempt to create the object, as it doesn't exist
		err = onCreate()

		if err != nil {
			logger.Error(err, fmt.Sprintf("Failed to create new %v: '%v/%v'", objectType, namespace, name))
			return ctrl.Result{}, err
		}

		// Object created successfully, so return and requeue
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get %v", objectType))
		return ctrl.Result{}, err
	}

	// Object exists and was successfully retrieved
	return ctrl.Result{}, nil
}

func (r *StroomClusterReconciler) getDatabaseConnectionInfo(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbRef *stroomv1.DatabaseRef, dbConnectionInfo *DatabaseConnectionInfo) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if dbRef.DatabaseServerRef == (stroomv1.ResourceRef{}) {
		// This is an external database connection
		dbConnectionInfo.Address = dbRef.ConnectionSpec.Address
		dbConnectionInfo.Port = dbRef.ConnectionSpec.Port
		dbConnectionInfo.SecretName = dbRef.ConnectionSpec.SecretName
	} else {
		// Get or create an operator-managed database instance
		dbServer := stroomv1.DatabaseServer{}
		dbReference := dbRef.DatabaseServerRef

		// If the DatabaseRef namespace is empty, try to find the DatabaseServer in the same namespace as StroomCluster
		if dbReference.Namespace == "" {
			dbReference.Namespace = stroomCluster.Namespace
		}

		if err := r.Get(ctx, types.NamespacedName{Namespace: dbReference.Namespace, Name: dbReference.Name}, &dbServer); err != nil {
			if errors.IsNotFound(err) {
				logger.Error(err, fmt.Sprintf("DatabaseServer '%v' was not found", dbReference))
			} else {
				logger.Error(err, fmt.Sprintf("Error accessing DatabaseServer '%v'", dbReference))
			}
			return ctrl.Result{}, err
		} else {
			if err := r.claimDatabaseServer(ctx, stroomCluster, dbReference, &dbServer); err != nil {
				return ctrl.Result{}, err
			}

			dbConnectionInfo.DatabaseServer = &dbServer
			dbConnectionInfo.Address = dbServer.GetServiceName()
			dbConnectionInfo.Port = DatabasePort
			dbConnectionInfo.SecretName = dbServer.GetSecretName()
		}
	}

	dbConnectionInfo.DatabaseName = dbRef.DatabaseName

	return ctrl.Result{}, nil
}

func (r *StroomClusterReconciler) claimDatabaseServer(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbRef stroomv1.ResourceRef, db *stroomv1.DatabaseServer) error {
	logger := log.FromContext(ctx)

	// If DatabaseServer is claimed by a StroomCluster, check whether it is the current cluster
	if db.StroomClusterRef != (stroomv1.ResourceRef{}) {
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

// SetupWithManager sets up the controller with the Manager.
func (r *StroomClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stroomv1.StroomCluster{}).
		Owns(&appsv1.StatefulSet{}).
		Complete(r)
}

func (r *StroomClusterReconciler) enableNodeSetServerTasks(ctx context.Context, dbInfo *DatabaseConnectionInfo, stroomCluster *stroomv1.StroomCluster, nodeSet *stroomv1.NodeSet, enabled bool) error {
	logger := log.FromContext(ctx)

	db, err := r.openDatabase(ctx, dbInfo, stroomCluster)
	if err != nil {
		return err
	}

	nodeName := stroomCluster.GetNodeSetName(nodeSet.Name)
	if result, err := db.Exec("update job_node set enabled=0 where node_name like ?", nodeName+"%"); err != nil {
		logger.Error(err, fmt.Sprintf("Could not set job enabled state to %v for NodeSet '%v'", enabled, nodeSet.Name))
		r.closeDatabase(db)
		return err
	} else {
		if rows, err := result.RowsAffected(); err != nil {
			logger.Error(err, "Failed to get number of rows affected")
			r.closeDatabase(db)
			return err
		} else {
			logger.Info(fmt.Sprintf("Set %v server tasks for node name '%v' to '%v'", rows, nodeName, enabled))
			r.closeDatabase(db)
		}
	}

	return nil
}