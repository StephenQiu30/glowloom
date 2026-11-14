package application

import (
	"slices"

	"github.com/google/uuid"
)

// ModeAvailability is the server's current permission to start work in one mode.
type ModeAvailability struct {
	Mode               string   `json:"mode"`
	Available          bool     `json:"available"`
	UnavailableReasons []string `json:"unavailable_reasons"`
}

// ModelAvailability is shared by administration and creation catalogs.
type ModelAvailability struct {
	Available          bool               `json:"available"`
	ModeAvailability   []ModeAvailability `json:"mode_availability"`
	UnavailableReasons []string           `json:"unavailable_reasons"`
	ReleaseID          uuid.UUID          `json:"availability_release_id"`
	ModelVersionID     uuid.UUID          `json:"availability_model_version_id"`
	PriceVersionID     uuid.UUID          `json:"availability_price_version_id"`
	ConfigVersionID    uuid.UUID          `json:"availability_config_version_id"`
	CredentialID       uuid.UUID          `json:"availability_credential_id"`
}

// ModelAvailabilityFacts contains only current, secret-free server evidence.
type ModelAvailabilityFacts struct {
	ReleaseID, ModelVersionID, PriceVersionID, ConfigVersionID, CredentialID  uuid.UUID
	Enabled, CredentialReady, ContractCompatible, PriceReady, VersionsCurrent bool
	Modes, VerifiedModes                                                      []string
}

// ProjectModelAvailability fails closed and never treats a credential probe as mode validation.
func ProjectModelAvailability(f ModelAvailabilityFacts) ModelAvailability {
	r := ModelAvailability{ModeAvailability: []ModeAvailability{}, UnavailableReasons: []string{}, ReleaseID: f.ReleaseID, ModelVersionID: f.ModelVersionID, PriceVersionID: f.PriceVersionID, ConfigVersionID: f.ConfigVersionID, CredentialID: f.CredentialID}
	checks := []struct {
		ok     bool
		reason string
	}{{f.Enabled, "disabled"}, {f.CredentialReady && f.CredentialID != uuid.Nil, "credential_required"}, {f.ReleaseID != uuid.Nil, "release_required"}, {f.ContractCompatible && f.ModelVersionID != uuid.Nil && f.ConfigVersionID != uuid.Nil, "unsupported_contract"}, {f.PriceReady && f.PriceVersionID != uuid.Nil, "price_unverified"}, {f.VersionsCurrent, "version_changed"}}
	for _, check := range checks {
		if !check.ok {
			r.UnavailableReasons = append(r.UnavailableReasons, check.reason)
		}
	}
	for _, mode := range f.Modes {
		reasons := append([]string{}, r.UnavailableReasons...)
		if !slices.Contains(f.VerifiedModes, mode) {
			reasons = append(reasons, "mode_unverified")
		}
		available := len(reasons) == 0
		r.ModeAvailability = append(r.ModeAvailability, ModeAvailability{Mode: mode, Available: available, UnavailableReasons: reasons})
		r.Available = r.Available || available
	}
	if !r.Available && len(r.UnavailableReasons) == 0 {
		r.UnavailableReasons = append(r.UnavailableReasons, "mode_unverified")
	}
	return r
}

// AvailableForMode applies the same mode projection used by public catalogs.
func (r ModelAvailability) AvailableForMode(mode string) bool {
	if mode == "" {
		return r.Available
	}
	return slices.ContainsFunc(r.ModeAvailability, func(m ModeAvailability) bool { return m.Mode == mode && m.Available })
}
