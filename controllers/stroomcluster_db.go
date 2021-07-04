package controllers

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	stroomv1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func (r *StroomClusterReconciler) openDatabase(ctx context.Context, dbInfo *DatabaseConnectionInfo, stroomCluster *stroomv1.StroomCluster) (*sql.DB, error) {
	logger := log.FromContext(ctx)

	databaseName := stroomCluster.Spec.AppDatabaseName

	// Get password from secret
	dbSecret := v1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: stroomCluster.Namespace, Name: dbInfo.SecretName}, &dbSecret); err != nil {
		logger.Error(err, fmt.Sprintf("Could not retrieve database password from Secret '%v'", dbInfo.SecretName))
		return nil, err
	}

	fqdn := fmt.Sprintf("%v.%v.svc.cluster.local", dbInfo.Address, stroomCluster.Namespace)
	password := string(dbSecret.Data[DatabaseServiceUserName])
	dataSourceName := fmt.Sprintf("%v:%v@tcp(%v:%v)/%v", DatabaseServiceUserName, password, fqdn, dbInfo.Port, databaseName)
	if db, err := sql.Open("mysql", dataSourceName); err != nil {
		logger.Error(err, "Could not connect to database", "HostName", fqdn, "Database", databaseName, "User", DatabaseServiceUserName)
		return nil, err
	} else {
		return db, nil
	}
}

func (r *StroomClusterReconciler) closeDatabase(database *sql.DB) {
	if err := database.Close(); err != nil {
		// Handle silently
	}
}
