package bible_test

import "testing"

func TestBibleMigrationPGCompleteImmutableRuntimeACLAndPinnedStructureColumn(t *testing.T) {
	db, owner := bibleTestDB(t)
	var count int64
	if err := owner.Raw(`SELECT count(*) FROM information_schema.tables WHERE table_schema='bible' AND table_type='BASE TABLE'`).Scan(&count).Error; err != nil || count != 20 {
		t.Fatal("incomplete Bible schema", count, err)
	}
	for _, table := range []string{"character_version", "character_confirmation", "location_version", "location_confirmation", "prop_version", "prop_confirmation", "look", "look_version", "reference_version", "voice_version", "character_redirect", "character_split", "command", "copy_snapshot", "copy_transfer_receipt", "copy_receipt", "copy_cleanup_receipt"} {
		name := "bible." + table
		var read, insert, update, remove bool
		if err := db.Raw(`SELECT has_table_privilege(current_user,?,'SELECT'),has_table_privilege(current_user,?,'INSERT'),has_table_privilege(current_user,?,'UPDATE'),has_table_privilege(current_user,?,'DELETE')`, name, name, name, name).Row().Scan(&read, &insert, &update, &remove); err != nil || !read || !insert || update || remove {
			t.Fatal("immutable runtime permissions", table, read, insert, update, remove, err)
		}
	}
	for _, table := range []string{"character", "location", "prop"} {
		var mutable, identity, remove bool
		if err := db.Raw(`SELECT has_column_privilege(current_user,?,'revision','UPDATE'),has_column_privilege(current_user,?,'id','UPDATE'),has_table_privilege(current_user,?,'DELETE')`, "bible."+table, "bible."+table, "bible."+table).Row().Scan(&mutable, &identity, &remove); err != nil || !mutable || identity || remove {
			t.Fatal("head-only mutation scope", table, mutable, identity, remove, err)
		}
	}
	var pinType string
	if err := owner.Raw(`SELECT data_type FROM information_schema.columns WHERE table_schema='script' AND table_name='dialogue_line' AND column_name='character_version_id'`).Scan(&pinType).Error; err != nil || pinType != "uuid" {
		t.Fatal("immutable script pin missing", pinType, err)
	}
	var canPin, canRewrite bool
	if err := db.Raw(`SELECT has_column_privilege(current_user,'script.dialogue_line','character_version_id','INSERT'),has_column_privilege(current_user,'script.dialogue_line','character_version_id','UPDATE')`).Row().Scan(&canPin, &canRewrite); err != nil || !canPin || canRewrite {
		t.Fatal("old dialogue history is mutable", canPin, canRewrite, err)
	}
	if err := db.Exec(`CREATE TABLE bible.unowned_runtime_ddl(id integer)`).Error; err == nil {
		t.Fatal("runtime unexpectedly owns schema DDL")
	}
}
