package identity_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// Reuse a deployed 007 only after checking its entire structural contract. An
// absent migration is installed inside the caller's rollback transaction only.
func verifyOrInstallMediaScope(t *testing.T, tx *gorm.DB, migration string) {
	t.Helper()
	var markers int64
	if err := tx.Raw(`SELECT
 (SELECT count(*) FROM information_schema.columns WHERE table_schema='media' AND table_name='transfer_job' AND column_name IN ('source_scope_id','target_scope_id','source_org_id','target_org_id'))+
 (SELECT count(*) FROM pg_constraint WHERE connamespace='media'::regnamespace AND (conname LIKE '%_actor_scope_user_fk' OR conname LIKE '%_physical_org_fk' OR conname IN ('transfer_source_scope_fkey','transfer_target_scope_fkey','transfer_source_org_fkey','transfer_target_org_fkey')))+
 (SELECT count(*) FROM pg_trigger WHERE tgrelid='media.transfer_job'::regclass AND tgname='transfer_ownership_immutable')+
 (SELECT count(*) FROM pg_proc WHERE oid=to_regprocedure('media.protect_transfer_ownership()'))+
 (SELECT count(*) FROM information_schema.columns WHERE table_schema='mediatool' AND table_name IN ('export_job','transcription_job','depth_job') AND column_name='actor_role' AND is_nullable='YES')`).Scan(&markers).Error; err != nil {
		t.Fatal(err)
	}
	if markers == 0 {
		if err := tx.Exec(migration).Error; err != nil {
			t.Fatal(err)
		}
	}
	var columns int64
	if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='media' AND table_name='transfer_job' AND column_name IN ('source_scope_id','target_scope_id','source_org_id','target_org_id') AND data_type='uuid' AND is_nullable='YES'`).Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	if columns != 4 {
		t.Fatal("partial or inconsistent media 007 transfer columns")
	}
	// Derive all required foreign keys from the reviewed migration itself, including
	// package tables when present; their absence remains a separate live preflight gap.
	keys := regexp.MustCompile(`ALTER TABLE (media\.[a-z_]+) ADD CONSTRAINT ([a-z_]+) FOREIGN KEY\(([a-z_]+)\) REFERENCES ([a-z_."]+)\(([a-z_]+)\);`).FindAllStringSubmatch(migration, -1)
	if len(keys) != 18 {
		t.Fatalf("unexpected media 007 foreign key contract: %d", len(keys))
	}
	for _, key := range keys {
		var exists bool
		if err := tx.Raw(`SELECT to_regclass(?) IS NOT NULL`, key[1]).Scan(&exists).Error; err != nil {
			t.Fatal(err)
		}
		if !exists && (key[1] == "media.package_command" || key[1] == "media.package_job") {
			continue
		}
		if !exists {
			t.Fatalf("required media table absent: %s", key[1])
		}
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conrelid=to_regclass(?) AND c.conname=? AND c.contype='f' AND c.convalidated AND c.confrelid=to_regclass(?) AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass(?) AND attname=?)]::smallint[] AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass(?) AND attname=?)]::smallint[] AND c.confupdtype='a' AND c.confdeltype='a')`, key[1], key[2], key[4], key[1], key[3], key[4], key[5]).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("partial or inconsistent media 007 constraint: %s", key[2])
		}
	}
	for _, table := range []string{"library_command", "package_command", "package_job", "purge_command", "purge_job", "transfer_command", "transfer_job"} {
		var legacy bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid=to_regclass(?) AND conname=?)`, "media."+table, table+"_org_id_actor_id_fkey").Scan(&legacy).Error; err != nil {
			t.Fatal(err)
		}
		if legacy {
			t.Fatalf("legacy actor ownership constraint retained: %s", table)
		}
	}
	var roles int64
	if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='mediatool' AND table_name IN ('export_job','transcription_job','depth_job') AND column_name='actor_role' AND is_nullable='YES'`).Scan(&roles).Error; err != nil {
		t.Fatal(err)
	}
	if roles != 3 {
		t.Fatal("partial media 007 durable actor columns")
	}
	functions := regexp.MustCompile(`(?s)CREATE OR REPLACE FUNCTION (media\.[a-z_]+)\(\) RETURNS trigger LANGUAGE plpgsql AS \$\$(.*?)\$\$;`).FindAllStringSubmatch(migration, -1)
	if len(functions) != 2 {
		t.Fatal("unexpected media 007 function contract")
	}
	for _, function := range functions {
		var body string
		if err := tx.Raw(`SELECT prosrc FROM pg_proc WHERE oid=to_regprocedure(?)`, function[1]+"()").Scan(&body).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(function[2]), " ") {
			t.Fatalf("partial or inconsistent media 007 function: %s", function[1])
		}
	}
	for _, trigger := range []struct {
		name, function string
		kind           int
	}{{"transfer_scope", "media.check_transfer_scope()", 7}, {"transfer_ownership_immutable", "media.protect_transfer_ownership()", 19}} {
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid='media.transfer_job'::regclass AND tgname=? AND tgfoid=to_regprocedure(?) AND tgtype=? AND tgenabled='O' AND NOT tgisinternal AND tgqual IS NULL)`, trigger.name, trigger.function, trigger.kind).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("partial or inconsistent media 007 trigger: %s", trigger.name)
		}
	}
}

// This is an authorization-regression fixture seam, not 008 migration acceptance.
// Only its two nullable reader columns are installed in the rollback transaction;
// no storage location, missing package table, or storage identity is fabricated.
func installMediaReviewStorageColumns(t *testing.T, tx *gorm.DB) {
	t.Helper()
	migration, err := os.ReadFile("../../db/migrations/008_object_storage_profiles.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"media_asset", "rendition"} {
		statement := "ALTER TABLE media." + table + " ADD COLUMN IF NOT EXISTS object_location_id uuid;"
		if !strings.Contains(string(migration), "\n"+statement+"\n") {
			t.Fatal("storage fixture column does not match migration 008")
		}
		if err := tx.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='media' AND table_name=? AND column_name='object_location_id' AND data_type='uuid' AND is_nullable='YES')`, table).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatal("inconsistent storage reader column")
		}
	}
}
