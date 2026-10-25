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

package controller

import (
	"context"

	batchv1 "k8s.io/api/batch/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
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
		return ctrl.Result{}, err
	}

	// Get connection information on the target database instance
	dbInfo := DatabaseConnectionInfo{}
	if err := GetDatabaseConnectionInfo(r.Client, ctx, &dbBackup.Spec.DatabaseServerRef, dbBackup.Namespace, &dbInfo); err != nil {
		return ctrl.Result{}, err
	}

	// Create or update the CronJob that performs scheduled database backups
	newCronJob := r.createCronJob(&dbBackup, &dbInfo)
	existingCronJob := batchv1.CronJob{
		ObjectMeta: metav1.ObjectMeta{
			Name:      newCronJob.Name,
			Namespace: newCronJob.Namespace,
		},
	}
	operationResult, err := controllerutil.CreateOrUpdate(ctx, r.Client, &existingCronJob, func() error {
		existingCronJob.Labels = newCronJob.Labels
		existingCronJob.Annotations = newCronJob.Annotations
		existingCronJob.OwnerReferences = newCronJob.OwnerReferences
		existingCronJob.Spec = newCronJob.Spec
		return nil
	})
	if err != nil {
		logger.Error(err, "Failed to reconcile CronJob", "Namespace", newCronJob.Namespace, "Name", newCronJob.Name)
		return ctrl.Result{}, err
	}
	logger.Info("CronJob reconciled", "Result", operationResult, "Namespace", existingCronJob.Namespace, "Name", existingCronJob.Name)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&stroomv1.DatabaseBackup{}).
		Owns(&batchv1.CronJob{}).
		Complete(r)
}
