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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
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
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=configmaps,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=statefulsets,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DatabaseServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dbServer := &stroomv1.DatabaseServer{}
	result := reconcile.Result{}

	err := r.Get(ctx, req.NamespacedName, dbServer)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("DatabaseServer resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
	}

	foundSecret := corev1.Secret{}
	result, err = r.getOrCreateObject(ctx, dbServer, "Secret", &foundSecret, func() error {
		// Generate a secret containing the root and service user passwords
		resource := r.createSecret(dbServer)
		logger.Info("Creating a new Secret", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	}

	foundConfigMap := corev1.Secret{}
	result, err = r.getOrCreateObject(ctx, dbServer, "ConfigMap", &foundConfigMap, func() error {
		// Generate a secret containing the root and service user passwords
		resource := r.createDbInitConfigMap(dbServer)
		logger.Info("Creating a new ConfigMap", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	}

	foundStatefulSet := appsv1.StatefulSet{}
	result, err = r.getOrCreateObject(ctx, dbServer, "StatefulSet", &foundStatefulSet, func() error {
		// Generate a secret containing the root and service user passwords
		resource := r.createStatefulSet(dbServer)
		logger.Info("Creating a new StatefulSet", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseServerReconciler) getOrCreateObject(ctx context.Context, dbServer *stroomv1.DatabaseServer, objectType string, foundObject client.Object, onCreate func() error) (reconcile.Result, error) {
	logger := log.FromContext(ctx)

	err := r.Get(ctx, types.NamespacedName{Name: dbServer.Name, Namespace: dbServer.Namespace}, foundObject)
	if err != nil && errors.IsNotFound(err) {
		// Attempt to create the object, as it doesn't exist
		err = onCreate()

		if err != nil {
			logger.Error(err, fmt.Sprintf("Failed to create new %v", objectType))
			return ctrl.Result{}, err
		}

		// Object does not exist, so create it
		return ctrl.Result{Requeue: true}, nil
	} else if err != nil {
		logger.Error(err, fmt.Sprintf("Failed to get %v", objectType))
		return ctrl.Result{}, err
	}

	// Object exists and was successfully retrieved
	return ctrl.Result{}, err
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stroomv1.DatabaseServer{}).
		Complete(r)
}
