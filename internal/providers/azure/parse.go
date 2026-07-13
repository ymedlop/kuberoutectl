package azure

import (
	"encoding/json"
	"fmt"
	"time"
)

// azAccount is the subset of `az account list` we consume. Fields we do not
// use are ignored by the JSON decoder.
type azAccount struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	State     string `json:"state"`
	TenantID  string `json:"tenantId"`
	IsDefault bool   `json:"isDefault"`
	User      struct {
		Name string `json:"name"`
		Type string `json:"type"`
	} `json:"user"`
}

// azCluster is the subset of an `az aks list` element we consume.
type azCluster struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	Location          string `json:"location"`
	ResourceGroup     string `json:"resourceGroup"`
	KubernetesVersion string `json:"kubernetesVersion"`
	Fqdn              string `json:"fqdn"`
	PowerState        struct {
		Code string `json:"code"`
	} `json:"powerState"`
	ProvisioningState string `json:"provisioningState"`
}

// azToken carries the resolved token expiry in UTC. ExpiresAt is the single
// source of truth for health mapping.
type azToken struct {
	ExpiresAt time.Time
}

// azTokenRaw mirrors `az account get-access-token`. Modern az emits the
// integer epoch `expires_on` (unambiguous UTC); older az emits only the local
// string `expiresOn`. We prefer the epoch and fall back to the string, in
// keeping with the repo rule of avoiding local-time ambiguity where possible.
type azTokenRaw struct {
	ExpiresOn      string `json:"expiresOn"`
	ExpiresOnEpoch *int64 `json:"expires_on"`
}

func parseAccounts(data []byte) ([]azAccount, error) {
	var accounts []azAccount
	if err := json.Unmarshal(data, &accounts); err != nil {
		return nil, fmt.Errorf("decode az account list: %w", err)
	}
	return accounts, nil
}

func parseAKSClusters(data []byte) ([]azCluster, error) {
	var clusters []azCluster
	if err := json.Unmarshal(data, &clusters); err != nil {
		return nil, fmt.Errorf("decode az aks list: %w", err)
	}
	return clusters, nil
}

// tokenLocalLayout is the format az uses for the deprecated local-time
// `expiresOn` string (microsecond precision, no zone).
const tokenLocalLayout = "2006-01-02 15:04:05.000000"

func parseAccessToken(data []byte) (azToken, error) {
	var raw azTokenRaw
	if err := json.Unmarshal(data, &raw); err != nil {
		return azToken{}, fmt.Errorf("decode az access token: %w", err)
	}
	if raw.ExpiresOnEpoch != nil {
		return azToken{ExpiresAt: time.Unix(*raw.ExpiresOnEpoch, 0).UTC()}, nil
	}
	if raw.ExpiresOn != "" {
		// Local-time fallback: interpret in the host location, then normalise
		// to UTC so everything downstream stays UTC.
		t, err := time.ParseInLocation(tokenLocalLayout, raw.ExpiresOn, time.Local)
		if err != nil {
			return azToken{}, fmt.Errorf("parse expiresOn %q: %w", raw.ExpiresOn, err)
		}
		return azToken{ExpiresAt: t.UTC()}, nil
	}
	return azToken{}, fmt.Errorf("access token has no expiry field")
}
