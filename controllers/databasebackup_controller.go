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
	"k8s.io/api/batch/v1beta1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
)

// DatabaseBackupReconciler reconciles a DatabaseBackup object
type DatabaseBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databasebackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databasebackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=stroom.gchq.github.io,resources=databasebackups/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.8.3/pkg/reconcile
func (r *DatabaseBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	dbBackup := stroomv1.DatabaseBackup{}
	if err := r.Get(ctx, req.NamespacedName, &dbBackup); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		logger.Error(err, "Unable to fetch DatabaseBackup", "Namespace", req.Namespace, "Name", req.Name)
	}

	// Get connection information on the target database instance
	dbInfo := DatabaseConnectionInfo{}
	if err := GetDatabaseConnectionInfo(r.Client, ctx, &dbBackup.Spec.DatabaseServerRef, dbBackup.Namespace, &dbInfo); err != nil {
		return ctrl.Result{}, err
	}

	foundCronJob := v1beta1.CronJob{}
	result, err := r.getOrCreateObject(ctx, dbBackup.GetBaseName(), dbBackup.Namespace, "CronJob", &foundCronJob, func() error {
		// Create a CronJob for performing scheduled database backups
		resource := r.createCronJob(&dbBackup, &dbInfo)
		logger.Info("Creating a new CronJob", "Namespace", resource.Namespace, "Name", resource.Name)
		return r.Create(ctx, resource)
	})
	if err != nil {
		return result, err
	} else if !result.IsZero() {
		return result, nil
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseBackupReconciler) getOrCreateObject(ctx context.Context, name string, namespace string, objectType string, foundObject client.Object, onCreate func() error) (reconcile.Result, error) {
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

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stroomv1.DatabaseBackup{}).
		Complete(r)
}
