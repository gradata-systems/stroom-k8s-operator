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
