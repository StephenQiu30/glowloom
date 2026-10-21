package config

import (
	"fmt"
	"time"
)

// ValidateIdentitySessionV2 permits verification-only rollback. Runtime must load
// configured verification keys even when new issuance is disabled.
func (c Config) ValidateIdentitySessionV2() error {
	if c.IdentitySessionV2Enabled && c.IdentityLegacyUpgradeUntil.IsZero() {
		return fmt.Errorf("%w: session v2 requires a fixed legacy upgrade deadline", ErrInvalid)
	}
	if !c.IdentityLegacyUpgradeUntil.IsZero() {
		_, offset := c.IdentityLegacyUpgradeUntil.Zone()
		if offset != 0 || c.IdentityLegacyUpgradeUntil.After(time.Now().UTC().Add(7*24*time.Hour)) {
			return fmt.Errorf("%w: legacy upgrade deadline must be UTC and at most seven days ahead", ErrInvalid)
		}
	}

	if c.IdentitySessionV2Enabled && (c.IdentityJWTPrivateKey == "" || c.IdentityJWTVerifyKeys == "" || c.IdentityJWTActiveKID == "" || c.IdentityRefreshReceiptKey == "") {
		return fmt.Errorf("%w: session v2 requires signing, verification and receipt keys", ErrInvalid)
	}
	if (c.IdentityJWTPrivateKey != "" || c.IdentityJWTActiveKID != "") && c.IdentityJWTVerifyKeys == "" {
		return fmt.Errorf("%w: retain access verification keys during rollback", ErrInvalid)
	}
	if c.IdentityJWTPrivateKey != "" && c.IdentityJWTPrivateKey == c.IdentityRefreshReceiptKey {
		return fmt.Errorf("%w: signing and receipt keys must be independent", ErrInvalid)
	}
	if len(c.IdentityJWTPrivateKey) > 16384 || len(c.IdentityJWTVerifyKeys) > 65536 || len(c.IdentityRefreshReceiptKey) > 128 {
		return fmt.Errorf("%w: identity session key configuration too large", ErrInvalid)
	}
	if len(c.IdentityJWTActiveKID) > 128 {
		return fmt.Errorf("%w: active key ID too long", ErrInvalid)
	}
	return nil
}
