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

package stroomcluster

import (
	"context"
	"fmt"
	"github.com/p-kimberley/stroom-k8s-operator/controllers/common"
	"github.com/p-kimberley/stroom-k8s-operator/controllers/databaseserver"
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
	appDatabaseConnectionInfo := databaseserver.DatabaseConnectionInfo{}
	if result, err := r.getDatabaseConnectionInfo(ctx, &stroomCluster, &appDatabaseRef, &appDatabaseConnectionInfo); err != nil {
		logger.Info(fmt.Sprintf("DatabaseServer '%v' could not be found", appDatabaseRef.DatabaseServerRef))
		return ctrl.Result{}, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	// Retrieve stats database connection info
	statsDatabaseRef := stroomCluster.Spec.StatsDatabaseRef
	statsDatabaseConnectionInfo := databaseserver.DatabaseConnectionInfo{}
	if result, err := r.getDatabaseConnectionInfo(ctx, &stroomCluster, &statsDatabaseRef, &statsDatabaseConnectionInfo); err != nil {
		return ctrl.Result{}, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	foundServiceAccount := corev1.ServiceAccount{}
	result, err := r.getOrCreateObject(ctx, GetBaseName(stroomCluster.Name), stroomCluster.Namespace, "ServiceAccount", &foundServiceAccount, func() error {
		// Create a new ServiceAccount
		resource := r.createServiceAccount(&stroomCluster)
		logger.Info("Creating a new ServiceAccount", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	// Check the StroomCluster ConfigMap exists
	foundConfigMap := corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{Name: stroomCluster.Spec.ConfigMapName, Namespace: stroomCluster.Namespace}, &foundConfigMap)
	if err != nil {
		logger.Error(err, fmt.Sprintf("ConfigMap '%v' referenced by StroomCluster '%v' was not found", stroomCluster.Spec.ConfigMapName, stroomCluster.Name))
		return ctrl.Result{}, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	// Query the StroomCluster StatefulSet and if it doesn't exist, create it
	for _, nodeSet := range stroomCluster.Spec.NodeSets {
		foundStatefulSet := appsv1.StatefulSet{}
		result, err = r.getOrCreateObject(ctx, GetStroomNodeSetName(stroomCluster.Name, nodeSet.Name), stroomCluster.Namespace, "StatefulSet", &foundStatefulSet, func() error {
			// Create a StatefulSet for the NodeSet
			resource := r.createStatefulSet(&stroomCluster, &nodeSet, &appDatabaseConnectionInfo, &statsDatabaseConnectionInfo)
			logger.Info("Creating a new StatefulSet", "Namespace", resource.Namespace, "Name", resource.Name)
			return r.Create(ctx, resource)
		})
		if err != nil {
			return result, err
		} else if result != (ctrl.Result{}) {
			return result, nil
		}

		// TODO: Update the replica count if different to the request

		foundService := corev1.Service{}
		result, err = r.getOrCreateObject(ctx, GetStroomNodeSetServiceName(stroomCluster.Name, nodeSet.Name), stroomCluster.Namespace, "Service", &foundService, func() error {
			// Create a headless service for the NodeSet
			resource := r.createService(&stroomCluster, &nodeSet)
			logger.Info("Creating a new Service", "Namespace", resource.Namespace, "Name", resource.Name)
			return r.Create(ctx, resource)
		})
		if err != nil {
			return result, err
		} else if result != (ctrl.Result{}) {
			return result, nil
		}
	}

	ingresses := r.createIngresses(&stroomCluster)
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

	if stroomCluster.ObjectMeta.DeletionTimestamp.IsZero() {
		if !common.ContainsString(stroomCluster.GetFinalizers(), common.FinalizerName) {
			// Finalizer hasn't been added, so add it to prevent the DatabaseServer from being deleted while the dependent StroomCluster still exists
			controllerutil.AddFinalizer(stroomCluster, common.FinalizerName)
			if err := r.Update(ctx, stroomCluster); err != nil {
				return false, err
			}
		}
	} else {
		if common.ContainsString(stroomCluster.GetFinalizers(), common.FinalizerName) {
			// TODO: Add deletion blocking logic

			// Remove finalizer from the linked DatabaseServers
			appDbResult := r.removeDatabaseFinalizer(ctx, stroomCluster, stroomCluster.Spec.AppDatabaseRef.DatabaseServerRef)
			statsDbResult := r.removeDatabaseFinalizer(ctx, stroomCluster, stroomCluster.Spec.StatsDatabaseRef.DatabaseServerRef)
			if appDbResult != nil && statsDbResult != nil {
				// A failure occurred, so block deletion until this resolves
				return true, nil
			}

			// Remove the finalizer, allowing the StroomCluster to be removed
			controllerutil.RemoveFinalizer(stroomCluster, common.FinalizerName)
			if err := r.Update(ctx, stroomCluster); err != nil {
				logger.Error(err, fmt.Sprintf("Finalizer could not be removed from StroomCluster '%v/%v'", stroomCluster.Namespace, stroomCluster.Name))
				return true, err
			}

			logger.Info(fmt.Sprintf("StroomCluster '%v/%v' deleted", stroomCluster.Namespace, stroomCluster.Name))
		}

		return true, nil
	}

	return false, nil
}

func (r *StroomClusterReconciler) removeDatabaseFinalizer(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbRef stroomv1.ResourceRef) error {
	logger := log.FromContext(ctx)
	db := stroomv1.DatabaseServer{}

	if dbRef == (stroomv1.ResourceRef{}) {
		return nil
	}

	// Use the StroomCluster namespace if none specified
	if dbRef.Namespace == "" {
		dbRef.Namespace = stroomCluster.Namespace
	}

	if err := r.Get(ctx, dbRef.NamespacedName(), &db); err == nil {
		controllerutil.RemoveFinalizer(&db, common.FinalizerName)
		if err := r.Update(ctx, &db); err == nil {
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

		// Object does not exist, so create it
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get %v", objectType))
		return ctrl.Result{}, err
	}

	// Object exists and was successfully retrieved
	return ctrl.Result{}, nil
}

func (r *StroomClusterReconciler) getDatabaseConnectionInfo(ctx context.Context, stroomCluster *stroomv1.StroomCluster, dbRef *stroomv1.DatabaseRef, dbConnectionInfo *databaseserver.DatabaseConnectionInfo) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if dbRef.DatabaseServerRef == (stroomv1.ResourceRef{}) {
		// This is an external database connection
		dbConnectionInfo.Address = dbRef.ConnectionSpec.Address
		dbConnectionInfo.Port = dbRef.ConnectionSpec.Port
		dbConnectionInfo.SecretName = dbRef.ConnectionSpec.SecretName
	} else {
		// Get or create an operator-managed database instance
		db := stroomv1.DatabaseServer{}
		dbReference := dbRef.DatabaseServerRef

		// If the DatabaseRef namespace is empty, try to find the DatabaseServer in the same namespace as StroomCluster
		if dbReference.Namespace == "" {
			dbReference.Namespace = stroomCluster.Namespace
		}

		if err := r.Get(ctx, types.NamespacedName{Namespace: dbReference.Namespace, Name: dbReference.Name}, &db); err != nil {
			if errors.IsNotFound(err) {
				logger.Error(err, fmt.Sprintf("DatabaseServer '%v' was not found", dbReference))
			} else {
				logger.Error(err, fmt.Sprintf("Error accessing DatabaseServer '%v'", dbReference))
			}
			return ctrl.Result{}, err
		} else {
			if err := r.claimDatabaseServer(ctx, stroomCluster, dbReference, &db); err != nil {
				return ctrl.Result{}, err
			}

			dbConnectionInfo.DatabaseServer = &db
			dbConnectionInfo.Address = databaseserver.GetServiceName(db.Name)
			dbConnectionInfo.Port = databaseserver.DatabasePort
			dbConnectionInfo.SecretName = databaseserver.GetSecretName(db.Name)
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
		Complete(r)
}
