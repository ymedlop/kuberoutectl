package aws

import (
	"encoding/json"
	"fmt"
	"strings"
)

// awsIdentity is the subset of `aws sts get-caller-identity` we consume.
type awsIdentity struct {
	UserID  string `json:"UserId"`
	Account string `json:"Account"`
	Arn     string `json:"Arn"`
}

// awsCluster is the subset of an EKS `describe-cluster` cluster object.
type awsCluster struct {
	Name     string `json:"name"`
	Arn      string `json:"arn"`
	Endpoint string `json:"endpoint"`
	Version  string `json:"version"`
	Status   string `json:"status"`
}

type awsEKSList struct {
	Clusters []string `json:"clusters"`
}

type awsEKSDescribe struct {
	Cluster awsCluster `json:"cluster"`
}

// parseProfiles splits `aws configure list-profiles` (newline-delimited plain
// text, not JSON) into trimmed, non-empty profile names.
func parseProfiles(data []byte) []string {
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		if s := strings.TrimSpace(line); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parseCallerIdentity(data []byte) (awsIdentity, error) {
	var id awsIdentity
	if err := json.Unmarshal(data, &id); err != nil {
		return awsIdentity{}, fmt.Errorf("decode sts get-caller-identity: %w", err)
	}
	return id, nil
}

func parseEKSList(data []byte) ([]string, error) {
	var list awsEKSList
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, fmt.Errorf("decode eks list-clusters: %w", err)
	}
	return list.Clusters, nil
}

func parseEKSDescribe(data []byte) (awsCluster, error) {
	var desc awsEKSDescribe
	if err := json.Unmarshal(data, &desc); err != nil {
		return awsCluster{}, fmt.Errorf("decode eks describe-cluster: %w", err)
	}
	return desc.Cluster, nil
}
