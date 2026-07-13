// Package services holds provider-agnostic business logic. Services depend on
// domain plus interfaces (registry, cache, execx) — never on a concrete
// provider package or on Cobra.
package services

import (
	"github.com/ymedlop/kuberoutectl/internal/execx"
	"github.com/ymedlop/kuberoutectl/internal/providers"
)

// CheckStatus is the outcome of a single doctor check.
type CheckStatus string

const (
	CheckOK   CheckStatus = "ok"
	CheckWarn CheckStatus = "warn"
	CheckFail CheckStatus = "fail"
)

// Check is one diagnostic result.
type Check struct {
	Name    string      `json:"name"`
	Status  CheckStatus `json:"status"`
	Detail  string      `json:"detail,omitempty"`
	Binary  string      `json:"binary,omitempty"`
	Version string      `json:"version,omitempty"`
}

// DoctorService inspects local prerequisites: which registered providers have
// their required CLI resolvable. It does not attempt discovery or network
// calls — it only answers "is the environment set up".
type DoctorService struct {
	registry *providers.Registry
	resolver execx.BinaryResolver
	// requiredBinary maps a provider ID to the CLI it needs. Kept here rather
	// than on the provider interface for the spine; a later slice can move it
	// onto Capabilities if warranted.
	requiredBinary map[string]string
}

// NewDoctorService builds a DoctorService.
func NewDoctorService(reg *providers.Registry, resolver execx.BinaryResolver, requiredBinary map[string]string) *DoctorService {
	return &DoctorService{registry: reg, resolver: resolver, requiredBinary: requiredBinary}
}

// Run executes all checks and returns them in deterministic order.
func (d *DoctorService) Run() []Check {
	var checks []Check
	for _, p := range d.registry.List() {
		id := string(p.ID())
		bin, ok := d.requiredBinary[id]
		if !ok || bin == "" {
			checks = append(checks, Check{
				Name:   "provider:" + id,
				Status: CheckOK,
				Detail: "no external CLI required",
			})
			continue
		}
		path, err := d.resolver.Resolve(bin)
		if err != nil {
			checks = append(checks, Check{
				Name:   "provider:" + id,
				Status: CheckFail,
				Binary: bin,
				Detail: err.Error(),
			})
			continue
		}
		checks = append(checks, Check{
			Name:   "provider:" + id,
			Status: CheckOK,
			Binary: bin,
			Detail: "resolved at " + path,
		})
	}
	return checks
}
