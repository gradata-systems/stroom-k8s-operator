package v1

type DatabaseServerRef struct {
	// If specified, point to an operator-managed DatabaseServer object
	// +optional
	ServerRef ResourceRef `json:"serverRef,omitempty"`

	// Alternatively, if the following parameters are provided, point directly to a DB by its TCP address.
	// This allows external database instances to be used in place of an operator-managed one.
	ServerAddress ServerAddress `json:"serverAddress,omitempty"`
}

type ServerAddress struct {
	// Host is the hostname or IP of the database server
	Host string `json:"host,omitempty"`
	// Port number the database server is listening on
	// +kubebuilder:default:=3306
	Port int32 `json:"port,omitempty"`
	// SecretName is the name of the secret containing the `password` of the database user `stroomuser`
	SecretName string `json:"secretName,omitempty"`
}
