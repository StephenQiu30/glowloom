package application

import "context"

// AuthPolicyStore supplies the operator's existing registration switch.
type AuthPolicyStore interface {
	ReadRegistrationEnabled(context.Context) (bool, error)
}

// AuthPolicy is public capability information, never a credential or authority grant.
type AuthPolicy struct {
	RegistrationEnabled                               bool
	TermsVersion, AvailableTermsVersion, ClosedReason string
}

// AuthPolicyQuery binds registration to an operator-accepted immutable agreement package.
type AuthPolicyQuery struct {
	store                               AuthPolicyStore
	configuredVersion, availableVersion string
	mailReady                           bool
}

// NewAuthPolicyQuery uses an already accepted version supplied by composition.
func NewAuthPolicyQuery(store AuthPolicyStore, configuredVersion, availableVersion string, mailReady bool) *AuthPolicyQuery {
	return &AuthPolicyQuery{store, configuredVersion, availableVersion, mailReady}
}

// Read returns the effective registration policy without changing settings.
func (q *AuthPolicyQuery) Read(ctx context.Context) (AuthPolicy, error) {
	enabled, err := q.store.ReadRegistrationEnabled(ctx)
	if err != nil {
		return AuthPolicy{}, err
	}
	p := AuthPolicy{AvailableTermsVersion: q.availableVersion}
	if q.configuredVersion != q.availableVersion {
		p.ClosedReason = "agreement_not_accepted"
		return p, nil
	}
	p.TermsVersion = q.configuredVersion
	if !q.mailReady {
		p.ClosedReason = "mail_unavailable"
		return p, nil
	}
	if !enabled {
		p.ClosedReason = "registration_closed"
		return p, nil
	}
	p.RegistrationEnabled = true
	return p, nil
}
