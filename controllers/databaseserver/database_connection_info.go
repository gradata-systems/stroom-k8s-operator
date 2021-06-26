package databaseserver

import (
	"fmt"
)

type DatabaseConnectionInfo struct {
	Address      string
	Port         int32
	DatabaseName string
	SecretName   string
}

func (dbInfo *DatabaseConnectionInfo) ToConnectionString() string {
	return fmt.Sprintf("jdbc:mysql://%v:%v/%v?useUnicode=yes&characterEncoding=UTF-8",
		dbInfo.Address, dbInfo.Port, dbInfo.DatabaseName)
}
