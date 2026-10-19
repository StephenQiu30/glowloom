package identity_test

import (
	"testing"

	"gorm.io/gorm"
)

// The connected database must already contain the complete schema.
func verifyMediaScope(t *testing.T, tx *gorm.DB) {
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
		t.Fatal("database is missing the media scope schema; initialize it from db/schema.sql")
	}
	var columns int64
	if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='media' AND table_name='transfer_job' AND column_name IN ('source_scope_id','target_scope_id','source_org_id','target_org_id') AND data_type='uuid' AND is_nullable='YES'`).Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	if columns != 4 {
		t.Fatal("partial or inconsistent media transfer columns")
	}
	keys := []struct{ table, name, column, refTable, refColumn string }{
		{"media.transfer_job", "transfer_source_scope_fkey", "source_scope_id", "workspace.creation_scope", "id"},
		{"media.transfer_job", "transfer_target_scope_fkey", "target_scope_id", "workspace.creation_scope", "id"},
		{"media.transfer_job", "transfer_source_org_fkey", "source_org_id", "workspace.organization", "id"},
		{"media.transfer_job", "transfer_target_org_fkey", "target_org_id", "workspace.organization", "id"},
		{"media.library_command", "library_command_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.library_command", "library_command_physical_org_fk", "org_id", "workspace.organization", "id"},
		{"media.package_command", "package_command_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.package_command", "package_command_physical_org_fk", "org_id", "workspace.organization", "id"},
		{"media.package_job", "package_job_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.package_job", "package_job_physical_org_fk", "org_id", "workspace.organization", "id"},
		{"media.purge_command", "purge_command_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.purge_command", "purge_command_physical_org_fk", "org_id", "workspace.organization", "id"},
		{"media.purge_job", "purge_job_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.purge_job", "purge_job_physical_org_fk", "org_id", "workspace.organization", "id"},
		{"media.transfer_command", "transfer_command_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.transfer_command", "transfer_command_physical_org_fk", "org_id", "workspace.organization", "id"},
		{"media.transfer_job", "transfer_job_actor_scope_user_fk", "actor_id", `identity."user"`, "id"},
		{"media.transfer_job", "transfer_job_physical_org_fk", "org_id", "workspace.organization", "id"},
	}
	for _, key := range keys {
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conrelid=to_regclass(?) AND c.conname=? AND c.contype='f' AND c.convalidated AND c.confrelid=to_regclass(?) AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass(?) AND attname=?)]::smallint[] AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=to_regclass(?) AND attname=?)]::smallint[] AND c.confupdtype='a' AND c.confdeltype='a')`, key.table, key.name, key.refTable, key.table, key.column, key.refTable, key.refColumn).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("partial or inconsistent media scope constraint: %s", key.name)
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
		t.Fatal("partial media durable actor columns")
	}
	var functions int64
	if err := tx.Raw(`SELECT count(*) FROM pg_proc WHERE oid IN (to_regprocedure('media.check_transfer_scope()'), to_regprocedure('media.protect_transfer_ownership()')) AND length(prosrc) > 0`).Scan(&functions).Error; err != nil {
		t.Fatal(err)
	}
	if functions != 2 {
		t.Fatal("media scope trigger functions are incomplete")
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
			t.Fatalf("partial or inconsistent media scope trigger: %s", trigger.name)
		}
	}
}

func installMediaReviewStorageColumns(t *testing.T, tx *gorm.DB) {
	t.Helper()
	for _, table := range []string{"media_asset", "rendition"} {
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='media' AND table_name=? AND column_name='object_location_id' AND data_type='uuid' AND is_nullable='YES')`, table).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatal("database is missing storage reader columns; initialize it from db/schema.sql")
		}
	}
}
