package prompt_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	promptpg "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/adapter/postgres"
	promptapp "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	workspacepg "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/adapter/postgres"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type draftAccess struct{ tx *gorm.DB }

func (a draftAccess) AuthorizeDraft(ctx context.Context, actor identityapp.Principal, p uuid.UUID, write bool) error {
	_, err := workspacepg.NewProjectContentAccessStore(a.tx).Authorize(ctx, actor, p, write)
	return err
}
func draftStore(db *gorm.DB) *promptpg.DraftStore {
	return promptpg.NewDraftStore(db, func(tx *gorm.DB) promptapp.DraftAccess { return draftAccess{tx} })
}
func TestEditorDraftPersistenceCASAndReplay(t *testing.T) {
	f := newPreferenceFixture(t)
	store := draftStore(f.db)
	input := promptapp.DraftWrite{ProjectID: f.project, Key: uuid.New(), Draft: editorDraft(t)}
	created, err := store.SaveDraft(t.Context(), f.actor, input)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := store.SaveDraft(t.Context(), f.actor, input)
	if err != nil || created.DraftID != replay.DraftID {
		t.Fatal("create replay", err)
	}
	in := promptapp.DraftWrite{ProjectID: f.project, DraftID: created.DraftID, ExpectedRevision: 1, Key: uuid.New(), Draft: editorDraft(t)}
	in.Draft.LabelState.HighWater["image"] = 1
	in.Draft.LabelState.Allocations["removed"] = domain.LabelAllocation{LabelKind: "image", Ordinal: 1}
	saved, err := store.SaveDraft(t.Context(), f.actor, in)
	if err != nil || saved.Revision != 2 || saved.DraftHash == created.DraftHash || saved.InputHash != created.InputHash {
		t.Fatal("metadata write", err, saved)
	}
	replay, err = store.SaveDraft(t.Context(), f.actor, in)
	if err != nil || replay.Revision != 2 {
		t.Fatal("replay before CAS", err)
	}
	in.Draft.Document.Content[0].Content[0].Text = "another body"
	if _, err = store.SaveDraft(t.Context(), f.actor, in); !errors.Is(err, promptapp.ErrIdempotencyConflict) {
		t.Fatal("changed body", err)
	}
	in.Key = uuid.New()
	if _, err = store.SaveDraft(t.Context(), f.actor, in); !errors.Is(err, promptapp.ErrRevisionConflict) {
		t.Fatal("CAS", err)
	}
	in.ExpectedRevision = 2
	in.Draft = editorDraft(t)
	if _, err = store.SaveDraft(t.Context(), f.actor, in); !errors.Is(err, domain.ErrLabelConflict) {
		t.Fatal("tombstones reclaimed", err)
	}
	actor := f.actor
	actor.ID = uuid.New()
	if _, err = store.ReadDraft(t.Context(), actor, f.project, created.DraftID); err == nil {
		t.Fatal("other actor read private draft")
	}
	list, err := store.ListDrafts(t.Context(), f.actor, f.project, "")
	if err != nil || len(list.Items) != 1 {
		t.Fatal("cross-device discovery", err)
	}
}
func TestEditorDraftConcurrentWriters(t *testing.T) {
	f := newPreferenceFixture(t)
	store := draftStore(f.db)
	created, err := store.SaveDraft(t.Context(), f.actor, promptapp.DraftWrite{ProjectID: f.project, Key: uuid.New(), Draft: editorDraft(t)})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, word := range []string{"甲", "乙"} {
		d := editorDraft(t)
		d.Document.Content[0].Content[0].Text = word
		wg.Go(func() {
			_, err := store.SaveDraft(t.Context(), f.actor, promptapp.DraftWrite{ProjectID: f.project, DraftID: created.DraftID, ExpectedRevision: 1, Key: uuid.New(), Draft: d})
			results <- err
		})
	}
	wg.Wait()
	close(results)
	ok, conflict := 0, 0
	for err := range results {
		if err == nil {
			ok++
		} else if errors.Is(err, promptapp.ErrRevisionConflict) {
			conflict++
		} else {
			t.Fatal(err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatal(ok, conflict)
	}
}
