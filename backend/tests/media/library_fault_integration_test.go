package media_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	pgmedia "github.com/StephenQiu30/lanverse/backend/internal/media/adapter/postgres"
	mediaapp "github.com/StephenQiu30/lanverse/backend/internal/media/application"
	"github.com/StephenQiu30/lanverse/backend/internal/media/domain"
)

func TestLibraryConcurrentIdenticalKeyHasOnePermanentAcceptance(t *testing.T) {
	db := libraryRuntimeDB(t)
	actor, _ := mediaStoreProject(t, db)
	store := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	text := "并发唯一"
	c := mediaapp.LibraryCommand{Scope: domain.LibraryScope{Kind: domain.LibraryPersonal}, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "唯一", Category: "other"}}
	var wg sync.WaitGroup
	results := make(chan mediaapp.LibraryReceipt, 8)
	failures := make(chan error, 8)
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := store.ApplyLibraryCommand(t.Context(), actor, c)
			if err != nil {
				failures <- err
				return
			}
			results <- result
		}()
	}
	wg.Wait()
	close(failures)
	close(results)
	for err := range failures {
		t.Error("concurrent permanent acceptance", err)
	}
	var first uuid.UUID
	count := 0
	for result := range results {
		count++
		if first == uuid.Nil {
			first = result.Items[0].ID
		}
		if result.Revision != 1 || result.Items[0].ID != first {
			t.Fatal("concurrent key produced multiple identities", result)
		}
	}
	if count != 8 {
		t.Fatal("missing concurrent result", count)
	}
	var receipts int64
	if err := db.Raw(`SELECT count(*) FROM media.library_command WHERE actor_id=? AND idem_key=?`, actor.ID, c.Key).Scan(&receipts).Error; err != nil || receipts != 1 {
		t.Fatal("permanent receipt count", receipts, err)
	}
}

func TestLibraryZeroRowReceiptRollsBackContentRevisionAndAudit(t *testing.T) {
	db := libraryRuntimeDB(t)
	owner := libraryOwnerDB(t)
	actor, _ := mediaStoreProject(t, db)
	store := pgmedia.NewLibraryStore(db, libraryTestAccess, time.Now)
	scope := domain.LibraryScope{Kind: domain.LibraryPersonal}
	library, _ := scope.Identity(actor.OrgID, actor.ID)
	function := "library_zero_" + uuid.NewString()
	function = strings.ReplaceAll(function, "-", "")
	create := fmt.Sprintf(`CREATE FUNCTION media.%s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.actor_id='%s'::uuid THEN RETURN NULL; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON media.library_command FOR EACH ROW EXECUTE FUNCTION media.%s()`, function, actor.ID, function, function)
	if err := owner.Exec(create).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Exec(fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON media.library_command; DROP FUNCTION IF EXISTS media.%s()`, function, function)).Error; err != nil {
			t.Error("remove exact fault trigger", err)
		}
	})
	text := "不能半提交"
	c := mediaapp.LibraryCommand{Scope: scope, Key: uuid.New(), Action: "create_text", Metadata: &mediaapp.LibraryMetadata{PlainText: &text, Title: "失败", Category: "other"}}
	if _, err := store.ApplyLibraryCommand(t.Context(), actor, c); !errors.Is(err, mediaapp.ErrLibraryConflict) {
		t.Fatal("zero-row permanent receipt reported success", err)
	}
	for _, query := range []string{`SELECT count(*) FROM media.library WHERE id=?`, `SELECT count(*) FROM media.library_item WHERE library_id=?`, `SELECT count(*) FROM media.library_command WHERE library_id=?`, `SELECT count(*) FROM audit.audit_log WHERE object_type='media.library' AND object_id=?::text`} {
		var count int64
		if err := db.Raw(query, library).Scan(&count).Error; err != nil || count != 0 {
			t.Fatal("zero-row receipt partly committed", count, err)
		}
	}
}
