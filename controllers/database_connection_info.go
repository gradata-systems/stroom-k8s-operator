package controllers

import (
	"fmt"
	v1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
)

type DatabaseConnectionInfo struct {
	DatabaseServer *v1.DatabaseServer
	v1.ServerAddress
}

func (dbInfo *DatabaseConnectionInfo) ToJdbcConnectionString(databaseName string) string {
	return fmt.Sprintf("jdbc:mysql://%v:%v/%v?useUnicode=yes&characterEncoding=UTF-8",
		dbInfo.Address, dbInfo.Port, databaseName)
}
