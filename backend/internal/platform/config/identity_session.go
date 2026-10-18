package config

import (
	"fmt"
	"path/filepath"
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

	for _, p := range []string{c.IdentityJWTKeyFile, c.IdentityJWTVerifyKeysFile, c.IdentityRefreshReceiptKeyFile} {
		if p != "" && !filepath.IsAbs(p) {
			return fmt.Errorf("%w: session key files must be absolute", ErrInvalid)
		}
	}
	if c.IdentitySessionV2Enabled && (c.IdentityJWTKeyFile == "" || c.IdentityJWTVerifyKeysFile == "" || c.IdentityJWTActiveKID == "" || c.IdentityRefreshReceiptKeyFile == "") {
		return fmt.Errorf("%w: session v2 requires signing, verification and receipt keys", ErrInvalid)
	}
	if (c.IdentityJWTKeyFile != "" || c.IdentityJWTActiveKID != "") && c.IdentityJWTVerifyKeysFile == "" {
		return fmt.Errorf("%w: retain access verification keys during rollback", ErrInvalid)
	}
	for _, p := range []string{c.IdentityJWTKeyFile, c.IdentityRefreshReceiptKeyFile} {
		if p != "" && filepath.Clean(p) == filepath.Clean(c.IdentityKeyFile) {
			return fmt.Errorf("%w: identity key purposes must be independent", ErrInvalid)
		}
	}
	if c.IdentityJWTKeyFile != "" && filepath.Clean(c.IdentityJWTKeyFile) == filepath.Clean(c.IdentityRefreshReceiptKeyFile) {
		return fmt.Errorf("%w: signing and receipt keys must be independent", ErrInvalid)
	}
	if len(c.IdentityJWTActiveKID) > 128 {
		return fmt.Errorf("%w: active key ID too long", ErrInvalid)
	}
	return nil
}
