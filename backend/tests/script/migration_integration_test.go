package script_test

import "testing"

func TestScriptSchemaPGFullDDLAndRuntimeColumnACL(t *testing.T) {
	_, database := scriptTestDB(t)
	var tables int64
	if err := database.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='script' AND table_type='BASE TABLE'`).Scan(&tables).Error; err != nil || tables != 31 {
		t.Fatal("complete schema missing tables", tables, err)
	}
	for _, table := range []string{"script_source", "script_version", "version_source", "split_set", "episode_structure", "scene", "dialogue_line", "action_line", "split_confirmation", "command", "command_result", "object_intent", "review_command", "request", "source_control", "copy_snapshot", "copy_object_intent", "copy_receipt", "import_job", "import_file", "import_attempt", "import_file_result", "import_publication", "import_command"} {
		var insert, update, deleteAllowed bool
		if err := database.Raw(`SELECT has_table_privilege('lanverse_app',?,'INSERT'),has_table_privilege('lanverse_app',?,'UPDATE'),has_table_privilege('lanverse_app',?,'DELETE')`, "script."+table, "script."+table, "script."+table).Row().Scan(&insert, &update, &deleteAllowed); err != nil || !insert || update || deleteAllowed {
			t.Fatal("immutable runtime ACL", table, insert, update, deleteAllowed, err)
		}
	}
	for _, column := range []string{"revision", "status", "cancellation_requested", "reconciliation_requested", "needs_reconciliation", "io_owner_id", "io_state"} {
		var allowed bool
		if err := database.Raw(`SELECT has_column_privilege('lanverse_app','script.import_state',?,'UPDATE')`, column).Scan(&allowed).Error; err != nil || !allowed {
			t.Fatal("own state column denied", column, allowed, err)
		}
	}
	var immutable bool
	if err := database.Raw(`SELECT has_column_privilege('lanverse_app','script.import_state','job_id','UPDATE')`).Scan(&immutable).Error; err != nil || immutable {
		t.Fatal("worker may rebind job identity", immutable, err)
	}
}
