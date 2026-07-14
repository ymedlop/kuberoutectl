package kubeconfig

import (
	"encoding/json"
	"fmt"
)

// The structs below capture only the kubeconfig fields kuberoutectl consumes
// from `kubectl config view --raw -o json`. The secret material (cert/token
// data) is intentionally not stored — only its presence classifies auth type.

type kcConfig struct {
	CurrentContext string           `json:"current-context"`
	Clusters       []kcNamedCluster `json:"clusters"`
	Contexts       []kcNamedContext `json:"contexts"`
	Users          []kcNamedUser    `json:"users"`
}

type kcNamedCluster struct {
	Name    string    `json:"name"`
	Cluster kcCluster `json:"cluster"`
}

type kcCluster struct {
	Server string `json:"server"`
}

type kcNamedContext struct {
	Name    string    `json:"name"`
	Context kcContext `json:"context"`
}

type kcContext struct {
	Cluster   string `json:"cluster"`
	User      string `json:"user"`
	Namespace string `json:"namespace"`
}

type kcNamedUser struct {
	Name string `json:"name"`
	User kcUser `json:"user"`
}

// kcUser carries only the discriminators needed to classify an auth type. The
// data fields (…-data) are presence flags for us, not values to keep.
type kcUser struct {
	ClientCertificate     string          `json:"client-certificate"`
	ClientCertificateData string          `json:"client-certificate-data"`
	Token                 string          `json:"token"`
	TokenFile             string          `json:"tokenFile"`
	Username              string          `json:"username"`
	Password              string          `json:"password"`
	Exec                  *kcExec         `json:"exec"`
	AuthProvider          *kcAuthProvider `json:"auth-provider"`
}

type kcExec struct {
	Command string `json:"command"`
}

type kcAuthProvider struct {
	Name string `json:"name"`
}

// parseConfig decodes `kubectl config view --raw -o json`.
func parseConfig(data []byte) (kcConfig, error) {
	var cfg kcConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return kcConfig{}, fmt.Errorf("decode kubectl config view: %w", err)
	}
	return cfg, nil
}
