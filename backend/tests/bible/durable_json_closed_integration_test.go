package bible_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// Mutations are limited to each test's own UUID fixture and rolled back. All
// owning readers run as the actual nonowner, including caller-owned transactions.
func bibleDurableMutation(t *testing.T, owner *gorm.DB, seed, read func(*gorm.DB)) {
	t.Helper()
	tx := owner.WithContext(t.Context()).Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	defer func() {
		if err := tx.Rollback().Error; err != nil {
			t.Error(err)
		}
	}()
	seed(tx)
	if err := tx.Exec(`SET LOCAL ROLE lanverse_app`).Error; err != nil {
		t.Fatal(err)
	}
	var role string
	if err := tx.Raw(`SELECT current_user`).Scan(&role).Error; err != nil || role != "lanverse_app" {
		t.Fatal("durable readers require actual nonowner", role, err)
	}
	read(tx)
}

type bibleDurableSnapshotRow struct {
	Manifest, ManifestSHA256, ContentSHA256 string
}

func bibleDurableSnapshotBody(t *testing.T, db *gorm.DB, id uuid.UUID) bibleDurableSnapshotRow {
	t.Helper()
	var row bibleDurableSnapshotRow
	r := db.WithContext(t.Context()).Raw(`SELECT manifest::text AS manifest,manifest_sha256,content_sha256 FROM bible.copy_snapshot WHERE id=?`, id).Scan(&row)
	if r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("durable snapshot", r.Error, r.RowsAffected)
	}
	return row
}

func bibleDurableCopyFixture(t *testing.T, db, owner *gorm.DB) (identityapp.Principal, app.ProjectCopyBinding, workspaceapp.ProjectCopyAuthority, app.ProjectCopySnapshot) {
	t.Helper()
	actor, source := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	first, err := service.Change(t.Context(), actor, createCharacter(source))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Change(t.Context(), actor, app.Command{ProjectID: source, Kind: domain.KindCharacter, Action: "confirm", EntryID: first.EntryID, ExpectedRevision: first.Revision, Key: uuid.New(), RequestID: uuid.New()}); err != nil {
		t.Fatal(err)
	}
	binding, authority := bibleCopyFixture(t, owner, actor, source)
	snapshot := freezeBibleCopy(t, db, actor, binding, authority)
	if err := app.NewProjectCopy(bibleCopyStore(db, authority), nil).Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("actual empty-media transfer", err)
	}
	authority.Phase = "register"
	if err := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		_, err := bibleCopyStore(tx, authority).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal("actual complete history registration", err)
	}
	return actor, binding, authority, snapshot
}

func bibleDurableCopyChecks(t *testing.T, actor identityapp.Principal, binding app.ProjectCopyBinding, authority workspaceapp.ProjectCopyAuthority, snapshot app.ProjectCopySnapshot) []struct {
	name string
	read func(*gorm.DB) error
} {
	t.Helper()
	return []struct {
		name string
		read func(*gorm.DB) error
	}{
		{"manifest", func(tx *gorm.DB) error {
			_, err := bibleCopyStore(tx, authority).Manifest(t.Context(), actor, binding, snapshot)
			return err
		}},
		{"transfer", func(tx *gorm.DB) error {
			return app.NewProjectCopy(bibleCopyStore(tx, authority), nil).Transfer(t.Context(), actor, binding, snapshot)
		}},
		{"register", func(tx *gorm.DB) error {
			_, err := bibleCopyStore(tx, authority).Register(t.Context(), actor, binding, snapshot)
			return err
		}},
		{"verify", func(tx *gorm.DB) error {
			_, err := bibleCopyStore(tx, authority).Verify(t.Context(), actor, binding, snapshot)
			return err
		}},
		{"cleanup", func(tx *gorm.DB) error {
			cleanup := authority
			cleanup.Phase = "cleanup"
			return bibleCopyStore(tx, cleanup).Cleanup(t.Context(), actor, binding, snapshot)
		}},
	}
}

func TestBibleDurableClosedPGManifestUnknownFields(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, binding, authority, snapshot := bibleDurableCopyFixture(t, db, owner)
	original := bibleDurableSnapshotBody(t, owner, snapshot.ID)
	for _, path := range []string{"root", "source", "source_character"} {
		t.Run(path, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(original.Manifest), &raw); err != nil {
				t.Fatal(err)
			}
			target := raw
			if path != "root" {
				target = raw["source"].(map[string]any)
			}
			if path == "source_character" {
				target = target["versions"].([]any)[0].(map[string]any)["character"].(map[string]any)
			}
			target["unknown_reference"] = map[string]any{"media": map[string]any{"asset_id": uuid.NewString()}}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			for _, check := range bibleDurableCopyChecks(t, actor, binding, authority, snapshot) {
				t.Run(check.name, func(t *testing.T) {
					bibleDurableMutation(t, owner, func(tx *gorm.DB) {
						if err := tx.Exec(`UPDATE bible.copy_snapshot SET manifest=?::jsonb WHERE id=?`, string(encoded), snapshot.ID).Error; err != nil {
							t.Fatal(err)
						}
						if check.name == "cleanup" {
							if err := tx.Exec(`UPDATE workspace.project_copy_job SET status='cancel_requested',cancellation_requested=true,stage='cleanup' WHERE id=?`, binding.JobID).Error; err != nil {
								t.Fatal(err)
							}
						}
					}, func(tx *gorm.DB) {
						if err := check.read(tx); !errors.Is(err, domain.ErrCorruptHistory) {
							t.Errorf("unknown durable manifest accepted by %s: %v", check.name, err)
						}
					})
				})
			}
		})
	}
	if after := bibleDurableSnapshotBody(t, owner, snapshot.ID); after != original {
		t.Fatal("reader changed immutable manifest bytes or SHA")
	}
}

func TestBibleDurableClosedPGLegacyManifestNilAndProofsUnchanged(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, binding, authority, snapshot := bibleDurableCopyFixture(t, db, owner)
	original := bibleDurableSnapshotBody(t, owner, snapshot.ID)
	var expected app.CopyManifest
	if err := json.Unmarshal([]byte(original.Manifest), &expected); err != nil {
		t.Fatal(err)
	}
	manifest, err := bibleCopyStore(db, authority).Manifest(t.Context(), actor, binding, snapshot)
	if err != nil || !reflect.DeepEqual(manifest, expected) || manifest.Source.Versions[0].Result != nil || manifest.Source.Versions[0].Character.Voice != nil {
		t.Fatal("legal legacy null semantics changed", err)
	}
	if manifest.ContentSHA256 != snapshot.ContentSHA256 || original.ManifestSHA256 != snapshot.ManifestSHA256 || original.ContentSHA256 != snapshot.ContentSHA256 {
		t.Fatal("original proof changed")
	}
	for _, check := range bibleDurableCopyChecks(t, actor, binding, authority, snapshot) {
		t.Run(check.name, func(t *testing.T) {
			bibleDurableMutation(t, owner, func(tx *gorm.DB) {
				if check.name == "cleanup" {
					if err := tx.Exec(`UPDATE workspace.project_copy_job SET status='cancel_requested',cancellation_requested=true,stage='cleanup' WHERE id=?`, binding.JobID).Error; err != nil {
						t.Fatal(err)
					}
				}
			}, func(tx *gorm.DB) {
				if err := check.read(tx); err != nil {
					t.Fatal("legal durable manifest reader", err)
				}
			})
		})
	}
	if after := bibleDurableSnapshotBody(t, owner, snapshot.ID); after != original {
		t.Fatal("legal replay rewrote manifest bytes or SHA")
	}
}

func TestBibleDurableClosedPGLargeCompleteManifestPreservesOriginalContract(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, source := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	text := strings.Repeat("清", domain.MaxDescriptionScalars)
	for range 8 {
		command := createCharacter(source)
		command.Character.Description = text
		command.Character.Definition = domain.CharacterDefinition{Role: text, Appearance: text, Physique: text, Clothing: text, Personality: text, Props: text, ConsistencyPrompt: text, MultiViewPrompt: text, VoiceLanguage: text, VoiceAge: text, VoiceTimbre: text}
		if _, err := service.Change(t.Context(), actor, command); err != nil {
			t.Fatal("actual legal individual version", err)
		}
	}
	binding, authority := bibleCopyFixture(t, owner, actor, source)
	snapshot := freezeBibleCopy(t, db, actor, binding, authority)
	original := bibleDurableSnapshotBody(t, owner, snapshot.ID)
	if len(original.Manifest) <= domain.MaxVersionBytes {
		t.Fatal("complete manifest did not exceed individual version byte budget", len(original.Manifest))
	}
	t.Logf("actual complete original manifest bytes=%d, character versions=%d", len(original.Manifest), snapshot.Counts.CharacterVersions)
	if err := app.NewProjectCopy(bibleCopyStore(db, authority), nil).Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal("complete manifest transfer rejected legitimate old history", err)
	}
	authority.Phase = "register"
	if err := db.WithContext(t.Context()).Transaction(func(tx *gorm.DB) error {
		store := bibleCopyStore(tx, authority)
		if _, err := store.Register(t.Context(), actor, binding, snapshot); err != nil {
			return err
		}
		_, err := store.Verify(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal("complete manifest registration or reread rejected legitimate old history", err)
	}
	if after := bibleDurableSnapshotBody(t, owner, snapshot.ID); after != original {
		t.Fatal("large historical manifest bytes or SHA rewritten")
	}
}

type bibleDurableCommandRow struct {
	Response, RequestHash string
}

func bibleDurableCommandBody(t *testing.T, db *gorm.DB, actor identityapp.Principal, key uuid.UUID) bibleDurableCommandRow {
	t.Helper()
	var row bibleDurableCommandRow
	r := db.Raw(`SELECT response::text AS response,request_hash FROM bible.command WHERE actor_id=? AND request_id=?`, actor.ID, key).Scan(&row)
	if r.Error != nil || r.RowsAffected != 1 {
		t.Fatal("durable command", r.Error, r.RowsAffected)
	}
	return row
}

func TestBibleDurableClosedPGReceiptUnknownFields(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	command := createCharacter(project)
	if _, err := app.NewService(bibleStore(db), time.Now).Change(t.Context(), actor, command); err != nil {
		t.Fatal(err)
	}
	original := bibleDurableCommandBody(t, owner, actor, command.Key)
	for _, nested := range []bool{false, true} {
		name := "root"
		if nested {
			name = "nested_unknown_result"
		}
		t.Run(name, func(t *testing.T) {
			var raw map[string]any
			if err := json.Unmarshal([]byte(original.Response), &raw); err != nil {
				t.Fatal(err)
			}
			// Receipt's accepted fields are flat scalars and UUID pointers. A
			// nested object outside that contract must never be silently dropped.
			raw["unknown_result"] = "future receipt"
			if nested {
				raw["unknown_result"] = map[string]any{"media": map[string]any{"asset_id": uuid.NewString()}}
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			bibleDurableMutation(t, owner, func(tx *gorm.DB) {
				if err := tx.Exec(`UPDATE bible.command SET response=?::jsonb WHERE actor_id=? AND request_id=?`, string(encoded), actor.ID, command.Key).Error; err != nil {
					t.Fatal(err)
				}
			}, func(tx *gorm.DB) {
				if _, err := app.NewService(bibleStore(tx), time.Now).Change(t.Context(), actor, command); !errors.Is(err, domain.ErrCorruptHistory) {
					t.Errorf("unknown durable command response accepted: %v", err)
				}
			})
		})
	}
	if after := bibleDurableCommandBody(t, owner, actor, command.Key); after != original {
		t.Fatal("reader rewrote durable receipt bytes or request hash")
	}
}

func TestBibleDurableClosedPGLegacyReceiptPointerNullAndReplayUnchanged(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	create := createCharacter(project)
	service := app.NewService(bibleStore(db), time.Now)
	created, err := service.Change(t.Context(), actor, create)
	if err != nil {
		t.Fatal(err)
	}
	confirm := app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "confirm", EntryID: created.EntryID, ExpectedRevision: created.Revision, Key: uuid.New(), RequestID: uuid.New()}
	confirmed, err := service.Change(t.Context(), actor, confirm)
	if err != nil || confirmed.ConfirmedVersionID == nil {
		t.Fatal("actual pointer receipt", err)
	}
	for _, sample := range []struct {
		name    string
		command app.Command
		receipt app.Receipt
	}{{"nil", create, created}, {"confirmed_pointer", confirm, confirmed}} {
		t.Run(sample.name, func(t *testing.T) {
			original := bibleDurableCommandBody(t, owner, actor, sample.command.Key)
			replay, err := service.Change(t.Context(), actor, sample.command)
			if err != nil || !reflect.DeepEqual(replay, sample.receipt) {
				t.Fatal("legal original receipt replay changed", replay, err)
			}
			var raw map[string]any
			if err := json.Unmarshal([]byte(original.Response), &raw); err != nil {
				t.Fatal(err)
			}
			for _, field := range []string{"confirmed_version_id", "created_entry_id", "created_version_id", "redirect_id"} {
				if _, present := raw[field]; !present {
					raw[field] = nil
				}
			}
			encoded, err := json.Marshal(raw)
			if err != nil {
				t.Fatal(err)
			}
			bibleDurableMutation(t, owner, func(tx *gorm.DB) {
				if err := tx.Exec(`UPDATE bible.command SET response=?::jsonb WHERE actor_id=? AND request_id=?`, string(encoded), actor.ID, sample.command.Key).Error; err != nil {
					t.Fatal(err)
				}
			}, func(tx *gorm.DB) {
				before := bibleDurableCommandBody(t, tx, actor, sample.command.Key)
				replay, err := app.NewService(bibleStore(tx), time.Now).Change(t.Context(), actor, sample.command)
				if err != nil || !reflect.DeepEqual(replay, sample.receipt) {
					t.Fatal("legal explicit-null receipt replay changed", replay, err)
				}
				if after := bibleDurableCommandBody(t, tx, actor, sample.command.Key); after != before {
					t.Fatal("reader rewrote explicit-null receipt")
				}
			})
			if after := bibleDurableCommandBody(t, owner, actor, sample.command.Key); after != original {
				t.Fatal("reader rewrote original receipt")
			}
		})
	}
}

func TestBibleCompleteClosedJSONDocumentPreservesVersionBudgetBoundary(t *testing.T) {
	type node struct {
		Text     string `json:"text,omitempty"`
		Children []node `json:"children,omitempty"`
	}
	for _, sample := range []struct{ name, body string }{
		{"deep_complete_document", strings.Repeat(`{"children":[`, 66) + `{"text":"known"}` + strings.Repeat(`]}`, 66)},
		{"many_complete_facts", `{"children":[` + strings.Repeat(`{"text":"known"},`, 100000) + `{"text":"known"}]}`},
	} {
		t.Run(sample.name, func(t *testing.T) {
			var value node
			if err := domain.DecodeClosedJSON([]byte(sample.body), &value); !errors.Is(err, domain.ErrInvalidContent) {
				t.Fatal("single-version budgets were weakened", err)
			}
			if err := domain.DecodeClosedJSONDocument([]byte(sample.body), &value, len(sample.body)); err != nil {
				t.Fatal("complete document inherited a new per-version budget", err)
			}
		})
	}
	for _, sample := range []struct{ name, body string }{
		{"duplicate", `{"text":"a","text":"b"}`},
		{"nested_duplicate", `{"children":[{"text":"a","text":"b"}]}`},
		{"unknown", `{"unknown":{}}`},
		{"nested_unknown", `{"children":[{"unknown":{}}]}`},
		{"trailing", `{"text":"known"} {}`},
		{"invalid_utf8", "{\"text\":\"" + string([]byte{0xff}) + "\"}"},
	} {
		t.Run(sample.name, func(t *testing.T) {
			var value node
			if err := domain.DecodeClosedJSONDocument([]byte(sample.body), &value, len(sample.body)); !errors.Is(err, domain.ErrInvalidContent) {
				t.Fatal("complete document lost its closed JSON contract", err)
			}
		})
	}
	var value node
	if err := domain.DecodeClosedJSONDocument([]byte(`{"text":"known"}`), &value, 1); !errors.Is(err, domain.ErrInvalidContent) {
		t.Fatal("explicit caller byte contract ignored", err)
	}
}
