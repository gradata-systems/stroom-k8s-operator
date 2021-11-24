package controllers

import (
	"context"
	"database/sql"
	"fmt"
	_ "github.com/go-sql-driver/mysql"
	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func GetDatabaseConnectionInfo(client client.Client, ctx context.Context, dbRef *stroomv1.DatabaseServerRef, ownerNamespace string, dbConnectionInfo *DatabaseConnectionInfo) error {
	logger := log.FromContext(ctx)

	if dbRef.ServerRef == (stroomv1.ResourceRef{}) {
		// This is an external database connection
		dbConnectionInfo.DatabaseServer = nil
		dbConnectionInfo.Host = dbRef.ServerAddress.Host
		dbConnectionInfo.Port = dbRef.ServerAddress.Port
		dbConnectionInfo.SecretName = dbRef.ServerAddress.SecretName
	} else {
		// If the ServerRef namespace is empty, try to find the DatabaseServer in the same namespace as the owner
		// (e.g. StroomCluster)
		dbReference := &dbRef.ServerRef
		if dbReference.Namespace == "" {
			dbReference.Namespace = ownerNamespace
		}

		// Get or create an operator-managed database instance
		dbServer := stroomv1.DatabaseServer{}
		if err := client.Get(ctx, dbReference.NamespacedName(), &dbServer); err != nil {
			if errors.IsNotFound(err) {
				logger.Error(err, "DatabaseServer was not found", "Reference", dbReference)
			} else {
				logger.Error(err, "Error accessing DatabaseServer", "Reference", dbReference)
			}
			return err
		} else {
			dbConnectionInfo.DatabaseServer = &dbServer
			dbConnectionInfo.Host = dbServer.GetServiceFqdn()
			dbConnectionInfo.Port = DatabasePort
			dbConnectionInfo.SecretName = dbServer.GetSecretName()
		}
	}

	return nil
}

func OpenDatabase(client client.Reader, ctx context.Context, dbInfo *DatabaseConnectionInfo, secretNamespace string, databaseName string) (*sql.DB, error) {
	logger := log.FromContext(ctx)

	// Get password from secret
	dbSecret := v1.Secret{}
	if err := client.Get(ctx, types.NamespacedName{Namespace: secretNamespace, Name: dbInfo.SecretName}, &dbSecret); err != nil {
		logger.Error(err, fmt.Sprintf("Could not retrieve database password from Secret '%v'", dbInfo.SecretName))
		return nil, err
	}

	password := string(dbSecret.Data[DatabaseServiceUserName])
	dataSourceName := fmt.Sprintf("%v:%v@tcp(%v:%v)/%v", DatabaseServiceUserName, password, dbInfo.Host, dbInfo.Port, databaseName)
	if db, err := sql.Open("mysql", dataSourceName); err != nil {
		logger.Error(err, "Could not connect to database", "HostName", dbInfo.Host, "Database", databaseName, "User", DatabaseServiceUserName)
		return nil, err
	} else {
		return db, nil
	}
}

func CloseDatabase(database *sql.DB) {
	if err := database.Close(); err != nil {
		// Handle silently
	}
}
