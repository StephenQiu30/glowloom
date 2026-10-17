package identity_test

import (
	"os"
	"testing"

	"github.com/google/uuid"
)

func TestDarkThemeMigrationPreservesSavedPreferences(t *testing.T) {
	tx := existingIdentityTransaction(t)
	org := uuid.New()
	if err := tx.Exec(`INSERT INTO workspace.organization(id,name) VALUES(?,'theme default fixture')`, org).Error; err != nil {
		t.Fatal(err)
	}
	users := make(map[uuid.UUID]string)
	for _, theme := range []string{"light", "dark", "system"} {
		id := uuid.New()
		if err := tx.Exec(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash,theme) VALUES(?,?,?,'fixture','producer','fixture-hash',?)`, id, org, id.String(), theme).Error; err != nil {
			t.Fatal(err)
		}
		users[id] = theme
	}
	if err := tx.Exec(`ALTER TABLE identity."user" ALTER COLUMN theme SET DEFAULT 'system'`).Error; err != nil {
		t.Fatal(err)
	}
	migration, err := os.ReadFile("../../db/migrations/015_identity_default_dark_theme.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec(string(migration)).Error; err != nil {
		t.Fatal(err)
	}
	for id, want := range users {
		var got string
		if err := tx.Raw(`SELECT theme FROM identity."user" WHERE id=?`, id).Scan(&got).Error; err != nil || got != want {
			t.Fatalf("saved preference = %q, error = %v; want %q", got, err, want)
		}
	}
	id := uuid.New()
	var theme string
	if err := tx.Raw(`INSERT INTO identity."user"(id,org_id,login_name,display_name,role,password_hash) VALUES(?,?,?,'fixture','producer','fixture-hash') RETURNING theme`, id, org, id.String()).Scan(&theme).Error; err != nil || theme != "dark" {
		t.Fatalf("database default theme = %q, error = %v; want dark", theme, err)
	}
}
