package domain

import (
	"fmt"
	"regexp"
	"strings"
)

// SystemLabelPrefix is the reserved namespace for tool-derived labels. Users
// may not write keys under this prefix; discovery owns it exclusively.
const SystemLabelPrefix = "kuberoutectl.io/"

// Well-known system label keys.
const (
	LabelProvider = SystemLabelPrefix + "provider"
	LabelSource   = SystemLabelPrefix + "source"
	LabelHealth   = SystemLabelPrefix + "health"
	LabelPlatform = SystemLabelPrefix + "platform"
	LabelRegion   = SystemLabelPrefix + "region"
)

// Label naming follows Kubernetes-inspired rules: an optional DNS-subdomain
// prefix followed by a name segment, each segment being alphanumeric with
// internal '-', '_' or '.'. Values follow the same name rule and may be empty.
var labelSegment = regexp.MustCompile(`^[a-z0-9A-Z]([a-z0-9A-Z._-]{0,61}[a-z0-9A-Z])?$`)

const maxLabelSegment = 63

// ValidateUserLabelKey checks a user-supplied label key. It rejects keys in
// the reserved system namespace so user labels can never shadow or corrupt
// tool-owned metadata.
func ValidateUserLabelKey(key string) error {
	if strings.HasPrefix(key, SystemLabelPrefix) {
		return fmt.Errorf("label key %q uses reserved namespace %q", key, SystemLabelPrefix)
	}
	if _, reserved := reservedBareKeys[key]; reserved {
		return fmt.Errorf("label key %q is reserved and cannot be used as a user label", key)
	}
	return validateLabelKey(key)
}

// reservedBareKeys are bare (unprefixed) keys the selector engine computes from a
// target's state. Reserving them stops a user label from shadowing the computed
// value — critical for visibility, where a stray `hidden=false` label would
// otherwise hide a genuinely-hidden target from `-l hidden=true`.
var reservedBareKeys = map[string]struct{}{
	"visible": {},
	"hidden":  {},
}

// validateLabelKey validates key format only (no namespace policy).
func validateLabelKey(key string) error {
	if key == "" {
		return fmt.Errorf("label key must not be empty")
	}
	name := key
	if i := strings.Index(key, "/"); i >= 0 {
		prefix := key[:i]
		name = key[i+1:]
		if prefix == "" {
			return fmt.Errorf("label key %q has empty prefix", key)
		}
		if len(prefix) > 253 {
			return fmt.Errorf("label key prefix %q exceeds 253 characters", prefix)
		}
		for _, part := range strings.Split(prefix, ".") {
			if !labelSegment.MatchString(part) {
				return fmt.Errorf("label key prefix segment %q is invalid", part)
			}
		}
	}
	if len(name) > maxLabelSegment {
		return fmt.Errorf("label key name %q exceeds %d characters", name, maxLabelSegment)
	}
	if !labelSegment.MatchString(name) {
		return fmt.Errorf("label key name %q is invalid", name)
	}
	return nil
}

// ValidateLabelValue checks a label value. Empty values are permitted.
func ValidateLabelValue(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxLabelSegment {
		return fmt.Errorf("label value %q exceeds %d characters", value, maxLabelSegment)
	}
	if !labelSegment.MatchString(value) {
		return fmt.Errorf("label value %q is invalid", value)
	}
	return nil
}

// ValidateUserLabel validates both key (with namespace policy) and value.
func ValidateUserLabel(key, value string) error {
	if err := ValidateUserLabelKey(key); err != nil {
		return err
	}
	return ValidateLabelValue(value)
}
