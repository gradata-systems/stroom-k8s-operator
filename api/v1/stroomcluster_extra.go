package v1

// VolumeClaimDeletePolicy determines when Stroom node PersistentVolumeClaims are deleted. If unset, they're never
// deleted by the operator.
// +kubebuilder:validation:Enum=DeleteOnScaledownAndClusterDeletion;DeleteOnScaledownOnly
type VolumeClaimDeletePolicy string

const (
	// DeleteOnScaledownAndClusterDeletionPolicy deletes a node's PersistentVolumeClaim when its NodeSet is scaled
	// down, and every node's when the StroomCluster is deleted
	DeleteOnScaledownAndClusterDeletionPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownAndClusterDeletion"
	// DeleteOnScaledownOnlyPolicy deletes a node's PersistentVolumeClaim when its NodeSet is scaled down
	DeleteOnScaledownOnlyPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownOnly"
)

type HttpsSettings struct {
	// Name of the TLS Secret containing the nodes' certificate (`tls.crt`), its private key (`tls.key`) and the CA
	// certificate (`ca.crt`), from which each node's key store and trust store are created
	TlsSecretName string `json:"tlsSecretName"`
	// Password of the keystore and truststore
	TlsKeystorePasswordSecretRef SecretItem `json:"tlsKeystorePasswordSecret"`
}

func (in *HttpsSettings) IsZero() bool {
	if in == nil {
		return true
	}

	return in.TlsSecretName == "" || in.TlsKeystorePasswordSecretRef == SecretItem{}
}

type IngressSettings struct {
	// DNS name at which the application will be reached (e.g. stroom.example.com)
	HostName string `json:"hostName"`
	// Name of the TLS `Secret` containing the private key and server certificate for the `Ingress`. If omitted, the
	// Ingresses have no TLS section.
	// +kubebuilder:validation:Optional
	SecretName string `json:"secretName,omitempty"`
	// Ingress class name (e.g. nginx). If omitted, the cluster's default IngressClass is used.
	ClassName string `json:"className,omitempty"`
	// Override path type for all ingress resources as `ImplementationSpecific`
	PathTypeOverride bool `json:"pathTypeOverride,omitempty"`
}

type OpenIdConfiguration struct {
	// Name of the OpenID client
	ClientId string `json:"clientId"`
	// Details of the Secret containing the OpenID client secret
	ClientSecret SecretItem `json:"clientSecret"`
}

func (in *OpenIdConfiguration) IsZero() bool {
	if in == nil {
		return true
	}

	return in.ClientId == "" || in.ClientSecret == SecretItem{}
}

type SecretItem struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}
