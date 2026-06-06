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

func (in *HttpsSettings) IsZero() bool {
	if in == nil {
		return true
	}

	return in.TlsSecretName == "" || in.TlsKeystorePasswordSecretRef == SecretItem{}
}

type IngressSettings struct {
	// DNS name at which the application will be reached (e.g. stroom.example.com)
	HostName string `json:"hostName"`
	// Name of the TLS `Secret` containing the private key and server certificate for the `Ingress`
	// +kubebuilder:validation:Optional
	SecretName string `json:"secretName,omitempty"`
	// Ingress class name (e.g. nginx)
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
