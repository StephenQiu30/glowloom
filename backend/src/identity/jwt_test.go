package identity

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestJWTManagerIssuesAndValidatesAccessToken(t *testing.T) {
	manager, err := NewJWTManager(strings.Repeat("s", 32), "lanverse-test", "admin-test", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	userID, workspaceID, sessionID := uuid.New(), uuid.New(), uuid.New()
	issuedAt := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	raw, expiresAt, err := manager.Issue(SessionIssue{SessionID: sessionID, Identity: AuthIdentity{Account: Account{ID: userID}, Workspace: Workspace{ID: workspaceID}}}, issuedAt)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if claims.Subject != userID.String() || claims.WorkspaceID != workspaceID.String() || claims.SessionID != sessionID.String() {
		t.Fatalf("claims = %#v", claims)
	}
	if !claims.ExpiresAt.Time.Equal(expiresAt) {
		t.Fatalf("expires_at = %s, want %s", claims.ExpiresAt.Time, expiresAt)
	}
}

func TestJWTManagerRejectsWrongAudienceAndShortSecret(t *testing.T) {
	if _, err := NewJWTManager("short", "issuer", "audience", time.Minute); err == nil {
		t.Fatal("short secret should be rejected")
	}
	if _, err := NewJWTManager(insecureJWTSecretPlaceholder, "issuer", "audience", time.Minute); err == nil {
		t.Fatal("example placeholder secret should be rejected")
	}
	manager, err := NewJWTManager(strings.Repeat("s", 32), "issuer", "audience", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewJWTManager(strings.Repeat("s", 32), "issuer", "other-audience", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	raw, _, err := other.Issue(SessionIssue{SessionID: uuid.New(), Identity: AuthIdentity{Account: Account{ID: uuid.New()}, Workspace: Workspace{ID: uuid.New()}}}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Parse(raw); err == nil {
		t.Fatal("wrong audience should be rejected")
	}
}
