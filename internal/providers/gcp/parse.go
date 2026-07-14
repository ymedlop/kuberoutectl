package gcp

import (
	"encoding/json"
	"fmt"
)

// The structs below capture only the fields kuberoutectl consumes from gcloud's
// `--format=json` output.

// gcpConfig is `gcloud config list --format=json`: the active account/project.
type gcpConfig struct {
	Core struct {
		Account string `json:"account"`
		Project string `json:"project"`
	} `json:"core"`
}

// gcpAuthAccount is one entry of `gcloud auth list --format=json`.
type gcpAuthAccount struct {
	Account string `json:"account"`
	Status  string `json:"status"` // "ACTIVE" for the active account, else ""
}

// gcpProject is one entry of `gcloud projects list --format=json`.
type gcpProject struct {
	ProjectID      string `json:"projectId"`
	Name           string `json:"name"`
	ProjectNumber  string `json:"projectNumber"`
	LifecycleState string `json:"lifecycleState"`
}

// gcpCluster is one entry of `gcloud container clusters list --format=json`.
type gcpCluster struct {
	Name                 string `json:"name"`
	Location             string `json:"location"`
	Endpoint             string `json:"endpoint"`
	CurrentMasterVersion string `json:"currentMasterVersion"`
	Status               string `json:"status"`
	SelfLink             string `json:"selfLink"`
}

func parseConfig(data []byte) (gcpConfig, error) {
	var cfg gcpConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return gcpConfig{}, fmt.Errorf("decode gcloud config list: %w", err)
	}
	return cfg, nil
}

func parseAuthList(data []byte) ([]gcpAuthAccount, error) {
	var accounts []gcpAuthAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("decode gcloud auth list: %w", err)
	}
	return accounts, nil
}

func parseProjects(data []byte) ([]gcpProject, error) {
	var projects []gcpProject
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("decode gcloud projects list: %w", err)
	}
	return projects, nil
}

func parseClusters(data []byte) ([]gcpCluster, error) {
	var clusters []gcpCluster
	if err := json.Unmarshal(data, &clusters); err != nil {
		return nil, fmt.Errorf("decode gcloud container clusters list: %w", err)
	}
	return clusters, nil
}
