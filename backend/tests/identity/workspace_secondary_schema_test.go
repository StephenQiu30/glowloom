package identity_test

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// A deployed 009 is reusable only when every structural declaration matches.
// Partial deployment is not repaired by tests. Absent 009 is rollback-only.
func installSecondaryScope(t *testing.T, tx *gorm.DB) {
	t.Helper()
	raw, err := os.ReadFile("../../db/migrations/009_workspace_secondary_scope.sql")
	if err != nil {
		t.Fatal(err)
	}
	var markers int64
	if err := tx.Raw(`SELECT
 (SELECT count(*) FROM information_schema.columns WHERE table_schema='workspace' AND table_name IN ('project_folder','project_folder_command','project_folder_placement','project_copy_job') AND column_name='owner_scope_id')+
 (SELECT count(*) FROM pg_constraint WHERE connamespace='workspace'::regnamespace AND conname IN ('project_folder_scope_fk','project_folder_command_scope_fk','project_folder_placement_scope_fk','project_copy_job_scope_fk'))+
 (SELECT count(*) FROM pg_proc WHERE oid=to_regprocedure('workspace.validate_secondary_scope()'))+
 (SELECT count(*) FROM pg_trigger WHERE tgname IN ('project_copy_scope_binding','project_folder_placement_scope_binding') AND tgrelid IN ('workspace.project_copy_job'::regclass,'workspace.project_folder_placement'::regclass))+
 (SELECT count(*) FROM pg_indexes WHERE schemaname='workspace' AND indexname IN ('project_folder_scope_actor_idx','project_copy_scope_idx'))`).Scan(&markers).Error; err != nil {
		t.Fatal(err)
	}
	if markers == 0 {
		if err := tx.Exec(string(raw)).Error; err != nil {
			t.Fatal(err)
		}
	}
	var columns int64
	if err := tx.Raw(`SELECT count(*) FROM information_schema.columns WHERE table_schema='workspace' AND table_name IN ('project_folder','project_folder_command','project_folder_placement','project_copy_job') AND column_name='owner_scope_id' AND data_type='uuid' AND is_nullable='YES'`).Scan(&columns).Error; err != nil {
		t.Fatal(err)
	}
	if columns != 4 {
		t.Fatal("partial or inconsistent workspace 009 ownership columns")
	}
	for _, table := range []string{"project_folder", "project_folder_command", "project_folder_placement", "project_copy_job"} {
		key := table + "_scope_fk"
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM pg_constraint c WHERE c.conrelid=to_regclass(?) AND c.conname=? AND c.contype='f' AND c.convalidated AND c.confrelid='workspace.creation_scope'::regclass AND c.conkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=c.conrelid AND attname='owner_scope_id'),(SELECT attnum FROM pg_attribute WHERE attrelid=c.conrelid AND attname='org_id')]::smallint[] AND c.confkey=ARRAY[(SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='id'),(SELECT attnum FROM pg_attribute WHERE attrelid=c.confrelid AND attname='physical_scope_id')]::smallint[] AND c.confupdtype='a' AND c.confdeltype='a')`, "workspace."+table, key).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("inconsistent workspace 009 constraint %s", key)
		}
	}
	functions := regexp.MustCompile(`(?s)CREATE FUNCTION workspace.validate_secondary_scope\(\) RETURNS trigger LANGUAGE plpgsql AS \$\$(.*?)\$\$;`).FindStringSubmatch(string(raw))
	if len(functions) != 2 {
		t.Fatal("unexpected workspace 009 function declaration")
	}
	var body string
	if err := tx.Raw(`SELECT prosrc FROM pg_proc WHERE oid=to_regprocedure('workspace.validate_secondary_scope()')`).Scan(&body).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Join(strings.Fields(body), " ") != strings.Join(strings.Fields(functions[1]), " ") {
		t.Fatal("inconsistent workspace 009 function body")
	}
	for _, binding := range []struct{ table, name string }{{"project_copy_job", "project_copy_scope_binding"}, {"project_folder_placement", "project_folder_placement_scope_binding"}} {
		var valid bool
		if err := tx.Raw(`SELECT EXISTS(SELECT 1 FROM pg_trigger WHERE tgrelid=to_regclass(?) AND tgname=? AND tgfoid=to_regprocedure('workspace.validate_secondary_scope()') AND tgtype=23 AND tgenabled='O' AND NOT tgisinternal AND tgqual IS NULL)`, "workspace."+binding.table, binding.name).Scan(&valid).Error; err != nil {
			t.Fatal(err)
		}
		if !valid {
			t.Fatalf("inconsistent workspace 009 trigger %s", binding.name)
		}
	}
	for name, want := range map[string]string{
		"project_folder_scope_actor_idx": "CREATE INDEX project_folder_scope_actor_idx ON workspace.project_folder USING btree (owner_scope_id, actor_id, update_time DESC, id DESC)",
		"project_copy_scope_idx":         "CREATE INDEX project_copy_scope_idx ON workspace.project_copy_job USING btree (owner_scope_id, source_project_id, create_time DESC, id DESC)",
	} {
		var got string
		if err := tx.Raw(`SELECT pg_get_indexdef(indexrelid) FROM pg_index WHERE indexrelid=to_regclass(?) AND indisvalid AND indisready`, "workspace."+name).Scan(&got).Error; err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("inconsistent workspace 009 index %s", name)
		}
	}
}
