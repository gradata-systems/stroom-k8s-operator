package v1

type VolumeClaimDeletePolicy string

const (
	DeleteOnScaledownAndClusterDeletionPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownAndClusterDeletion"
	DeleteOnScaledownOnlyPolicy                                       = "DeleteOnScaledownOnly"
)

type IngressSettings struct {
	// DNS name at which the application will be reached (e.g. stroom.example.com)
	HostName string `json:"hostName"`
	// Name of the TLS `Secret` containing the private key and server certificate for the `Ingress`
	SecretName string `json:"secretName"`
	// Ingress class name (e.g. nginx)
	ClassName string `json:"className,omitempty"`
}

type DatabaseServerRef struct {
	// If specified, point to an operator-managed DatabaseServer object
	// +optional
	ServerRef ResourceRef `json:"serverRef,omitempty"`

	// Alternatively, if the following parameters are provided, point directly to a DB by its TCP address.
	// This allows external database instances to be used in place of an operator-managed one.
	ServerAddress ServerAddress `json:"serverAddress,omitempty"`
}

type ServerAddress struct {
	// Address is the hostname or IP of the database server
	Address string `json:"address,omitempty"`
	// Port number the database server is listening on
	// +kubebuilder:default:=3306
	Port int32 `json:"port,omitempty"`
	// SecretName is the name of the secret containing the password of both the `root` and `stroomuser` users
	// in the database instance
	SecretName string `json:"secretName,omitempty"`
}
