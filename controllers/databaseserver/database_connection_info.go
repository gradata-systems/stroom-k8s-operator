package databaseserver

import (
	"fmt"
	v1 "github.com/p-kimberley/stroom-k8s-operator/api/v1"
)

type DatabaseConnectionInfo struct {
	DatabaseRef  *v1.DatabaseServer
	Address      string
	Port         int32
	DatabaseName string
	SecretName   string
}

func (dbInfo *DatabaseConnectionInfo) ToConnectionString() string {
	return fmt.Sprintf("jdbc:mysql://%v:%v/%v?useUnicode=yes&characterEncoding=UTF-8",
		dbInfo.Address, dbInfo.Port, dbInfo.DatabaseName)
}
