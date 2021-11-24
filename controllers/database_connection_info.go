package controllers

import (
	"fmt"
	v1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

type DatabaseConnectionInfo struct {
	DatabaseServer *v1.DatabaseServer
	v1.ServerAddress
}

func (dbInfo *DatabaseConnectionInfo) ToJdbcConnectionString(databaseName string) string {
	return fmt.Sprintf("jdbc:mysql://%v:%v/%v?serverTimezone=UTC&useUnicode=yes&characterEncoding=UTF-8",
		dbInfo.Host, dbInfo.Port, databaseName)
}
