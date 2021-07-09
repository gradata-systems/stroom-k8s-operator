package v1

type VolumeClaimDeletePolicy string

const (
	DeleteOnScaledownAndClusterDeletionPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownAndClusterDeletion"
	DeleteOnScaledownOnlyPolicy                                       = "DeleteOnScaledownOnly"
)

type IngressSettings struct {
	HostName   string `json:"hostName"`
	SecretName string `json:"secretName"`
	ClassName  string `json:"className,omitempty"`
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
	Address    string `json:"address,omitempty"`
	Port       int32  `json:"port,omitempty"`
	SecretName string `json:"secretName,omitempty"`
}
