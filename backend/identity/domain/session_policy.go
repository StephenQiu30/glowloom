package domain

import "time"

// SessionPolicy is the server-selected lifetime for a newly created v2 session.
// Existing sessions retain their stored deadlines and protocol semantics.
type SessionPolicy struct {
	IdleTTL     time.Duration
	AbsoluteTTL time.Duration
}

// SessionPolicyFor selects the accepted v2 lifetime for the login preference.
func SessionPolicyFor(persistent bool) SessionPolicy {
	if persistent {
		return SessionPolicy{IdleTTL: 7 * 24 * time.Hour, AbsoluteTTL: 30 * 24 * time.Hour}
	}
	return SessionPolicy{IdleTTL: 12 * time.Hour, AbsoluteTTL: 7 * 24 * time.Hour}
}
