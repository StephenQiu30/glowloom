package identity_test

import (
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/StephenQiu30/glowloom-server/backend/internal/app"
	"github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/credentialschema"
	pgcatalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/adapter/postgres"
	catalog "github.com/StephenQiu30/glowloom-server/backend/internal/catalog/application"
	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/tests/identityfixture"
)

func TestCatalogFixtureUsesVerifiedSessionAndCurrentPlatformAuthority(t *testing.T) {
	tx := existingIdentityTransaction(t)
	actor := identity.Principal{ID: uuid.New(), OrgID: uuid.New(), PlatformRole: "platform_admin"}
	if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,platform_role) VALUES(?,?,?,'Catalog fixture','producer','synthetic','platform_admin')`, actor.ID, actor.OrgID, "catalog-fixture-"+actor.ID.String()).Error; err != nil {
		t.Fatal(err)
	}
	provider := uuid.New()
	if err := tx.Exec(`INSERT INTO catalog.provider(id,key,name,adapter_key,region) VALUES(?,?,'Fixture','openrouter','overseas')`, provider, "fixture-"+provider.String()).Error; err != nil {
		t.Fatal(err)
	}
	query := func(store *pgcatalog.Store, a identity.Principal) error {
		_, err := catalog.NewProviderDetailQuery(store, credentialschema.NewRegistry()).Execute(t.Context(), a, provider)
		return err
	}
	if err := query(pgcatalog.NewStore(tx), actor); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("missing dependency did not fail closed", err)
	}
	if err := query(app.NewCatalogStore(tx), actor); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("legacy principal admitted without a session", err)
	}
	actor = identityfixture.Session(t, tx, actor)
	if err := query(app.NewCatalogStore(tx), actor); err != nil {
		t.Fatal("verified fixture rejected", err)
	}
	stale := actor
	stale.CredentialEpoch++
	if err := query(app.NewCatalogStore(tx), stale); !errors.Is(err, identity.ErrSessionInvalid) {
		t.Fatal("stale session admitted", err)
	}
	if err := tx.Exec(`UPDATE identity."user" SET platform_role='creator' WHERE id=?`, actor.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := query(app.NewCatalogStore(tx), actor); !errors.Is(err, identity.ErrForbidden) {
		t.Fatal("revoked administrator admitted", err)
	}
}
