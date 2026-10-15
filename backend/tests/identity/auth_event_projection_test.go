package identity_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/credentialproof"
	pgidentity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/adapter/postgres"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
)

func TestEmailAuthenticationAppearsInSecurityActivity(t *testing.T) {
	tx := existingIdentityTransaction(t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	auth, actor := verifiedTestActor(t, tx, "auth-activity@example.invalid", now)
	key, err := credentialproof.New("test-only", []byte(strings.Repeat("h", 32)), []byte(strings.Repeat("e", 32)))
	if err != nil {
		t.Fatal(err)
	}
	query := identity.NewEventQuery(pgidentity.NewEventStore(tx), key, func() time.Time { return now })
	page, err := query.List(t.Context(), actor, identity.EventFilter{ScopeKind: "self", Action: "session.created"})
	if err != nil || len(page.Items) != 1 {
		t.Fatal("email login missing from security activity")
	}
	if _, err := auth.Login(t.Context(), "auth-activity@example.invalid", "wrong password", false); !errors.Is(err, identity.ErrInvalidCredentials) {
		t.Fatal("bad credential accepted")
	}
	page, err = query.List(t.Context(), actor, identity.EventFilter{ScopeKind: "self", Action: "authentication.failed"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Result != "failure" || page.Items[0].ActorID != nil {
		t.Fatal("failed login absent or attributed to verified actor")
	}
	var legacyCount int
	if err := tx.Raw(`SELECT count(*) FROM identity.auth_event WHERE account_id=?`, actor.ID).Scan(&legacyCount).Error; err != nil || legacyCount != 0 {
		t.Fatal("new email flow still writes legacy auth event stream")
	}
}
