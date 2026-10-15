package objectstorage

import (
	"fmt"
	"time"
)

// Ref is a complete physical location. A bare key never selects a default profile.
type Ref struct {
	ProfileID string `json:"profile_id"`
	Bucket    string `json:"bucket"`
	Key       string `json:"key"`
}

// AccessGrant is an SDK-generated private URL with its actual bounded lifetime.
type AccessGrant struct {
	URL       string    `json:"-" yaml:"-"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Format keeps private URLs out of generic diagnostics.
func (g AccessGrant) Format(state fmt.State, _ rune) {
	_, _ = fmt.Fprintf(state, "[private access grant expires %s]", g.ExpiresAt.UTC().Format(time.RFC3339))
}
