package v1

type VolumeClaimDeletePolicy string

const (
	DeleteOnScaledownAndClusterDeletionPolicy VolumeClaimDeletePolicy = "DeleteOnScaledownAndClusterDeletion"
	DeleteOnScaledownOnlyPolicy                                       = "DeleteOnScaledownOnly"
)

type HttpsSettings struct {
	// Name of the TLS secret containing the items `keystore.p12` and `truststore.p12`
	TlsSecretName string `json:"tlsSecretName"`
	// Password of the keystore and truststore
	TlsKeystorePasswordSecretRef SecretItem `json:"tlsKeystorePasswordSecret"`
}

type IngressSettings struct {
	// DNS name at which the application will be reached (e.g. stroom.example.com)
	HostName string `json:"hostName"`
	// Name of the TLS `Secret` containing the private key and server certificate for the `Ingress`
	SecretName string `json:"secretName"`
	// Ingress class name (e.g. nginx)
	ClassName string `json:"className,omitempty"`
}

type OpenIdConfiguration struct {
	// Name of the OpenID client
	ClientId string `json:"clientId"`
	// Details of the Secret containing the OpenID client secret
	ClientSecret SecretItem `json:"clientSecret"`
}

type SecretItem struct {
	SecretName string `json:"secretName"`
	Key        string `json:"key"`
}
