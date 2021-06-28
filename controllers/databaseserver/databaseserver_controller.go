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

package databaseserver

import (
	"context"
	"fmt"
	"github.com/p-kimberley/stroom-k8s-operator/controllers/common"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
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

// DatabaseServerReconciler reconciles a DatabaseServer object
type DatabaseServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databaseservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databaseservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databaseservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=stroomclusters,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=serviceaccounts,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DatabaseServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dbServer := stroomv1.DatabaseServer{}
	result := reconcile.Result{}

	if err := r.Get(ctx, req.NamespacedName, &dbServer); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, fmt.Sprintf("Unable to fetch DatabaseServer %v", req.NamespacedName.String()))
		return ctrl.Result{}, err
	}

	// If item is deleted or an error occurs when adding a finalizer, bail out
	if deleted, err := r.checkIfDeleted(ctx, &dbServer); deleted || err != nil {
		return ctrl.Result{}, err
	}

	foundSecret := corev1.Secret{}
	result, err := r.getOrCreateObject(ctx, GetSecretName(dbServer.Name), dbServer.Namespace, "Secret", &foundSecret, func() error {
		// Generate a Secret containing the root and service user passwords
		resource := r.createSecret(&dbServer)
		logger.Info("Creating a new Secret", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	foundConfigMap := corev1.ConfigMap{}
	result, err = r.getOrCreateObject(ctx, GetConfigMapName(dbServer.Name), dbServer.Namespace, "ConfigMap", &foundConfigMap, func() error {
		// Generate a ConfigMap containing the MySQL database configuration
		resource := r.createConfigMap(&dbServer)
		logger.Info("Creating a new ConfigMap", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	foundInitConfigMap := corev1.ConfigMap{}
	result, err = r.getOrCreateObject(ctx, GetInitConfigMapName(dbServer.Name), dbServer.Namespace, "ConfigMap", &foundInitConfigMap, func() error {
		// Generate a ConfigMap containing database initialisation scripts
		resource := r.createDbInitConfigMap(&dbServer)
		logger.Info("Creating a new ConfigMap", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	foundStatefulSet := appsv1.StatefulSet{}
	result, err = r.getOrCreateObject(ctx, GetBaseName(dbServer.Name), dbServer.Namespace, "StatefulSet", &foundStatefulSet, func() error {
		// Generate a StatefulSet for running a single instance of MySQL
		resource := r.createStatefulSet(&dbServer)
		logger.Info("Creating a new StatefulSet", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	foundService := corev1.Service{}
	result, err = r.getOrCreateObject(ctx, GetServiceName(dbServer.Name), dbServer.Namespace, "Service", &foundService, func() error {
		// Create a headless service
		resource := r.createService(&dbServer)
		logger.Info("Creating a new Service", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if result != (ctrl.Result{}) {
		return result, nil
	}

	return ctrl.Result{}, nil
}

// checkIfDeleted inspects the DatabaseServer to see if a deletion request is pending.
// If true, it executes finalizer logic to block deletion while the associated StroomCluster (if defined) still exists.
// Returns whether the resource is being deleted and any error that occured.
func (r *DatabaseServerReconciler) checkIfDeleted(ctx context.Context, dbServer *stroomv1.DatabaseServer) (bool, error) {
	logger := log.FromContext(ctx)

	const finalizerName = "stroom.gchq.github.io/finalizer"

	if dbServer.ObjectMeta.DeletionTimestamp.IsZero() {
		if !common.ContainsString(dbServer.GetFinalizers(), finalizerName) {
			// Finalizer hasn't been added, so add it to prevent the DatabaseServer from being deleted while the dependent StroomCluster still exists
			controllerutil.AddFinalizer(dbServer, finalizerName)
			if err := r.Update(ctx, dbServer); err != nil {
				return false, err
			}
		}
	} else {
		if common.ContainsString(dbServer.GetFinalizers(), finalizerName) {
			// Finalizer is present, so check whether the DatabaseServer is claimed by a StroomCluster
			if dbServer.StroomClusterRef != (stroomv1.StroomClusterRef{}) {
				stroomCluster := stroomv1.StroomCluster{}
				if err := r.Get(ctx, types.NamespacedName{Name: dbServer.StroomClusterRef.Name, Namespace: dbServer.StroomClusterRef.Namespace}, &stroomCluster); err == nil {
					// Related StroomCluster resource exists, so block deletion
					logger.Info(fmt.Sprintf("DatabaseServer will be deleted once StroomCluster '%v/%v' is deleted", stroomCluster.Namespace, stroomCluster.Name))
					return true, nil
				}
			}

			// Not claimed by a StroomCluster or the StroomCluster doesn't exist, so remove the finalizer.
			// This allows the DatabaseServer resource to be removed.
			controllerutil.RemoveFinalizer(dbServer, finalizerName)
			if err := r.Update(ctx, dbServer); err != nil {
				logger.Error(err, fmt.Sprintf("Finalizer could not be removed from DatabaseServer '%v/%v'", dbServer.Namespace, dbServer.Name))
				return true, err
			}

			logger.Info(fmt.Sprintf("DatabaseServer '%v/%v' deleted", dbServer.Namespace, dbServer.Name))
		}
	}

	return false, nil
}

func (r *DatabaseServerReconciler) getOrCreateObject(ctx context.Context, name string, namespace string, objectType string, foundObject client.Object, onCreate func() error) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, foundObject)
	if err != nil && errors.IsNotFound(err) {
		// Attempt to create the object, as it doesn't exist
		err = onCreate()

		if err != nil {
			logger.Error(err, fmt.Sprintf("Failed to create new %v: %v/%v", objectType, namespace, name))
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

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stroomv1.DatabaseServer{}).
		Complete(r)
}
