package bible_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	pg "github.com/StephenQiu30/lanverse/backend/internal/bible/adapter/postgres"
	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	workspacepg "github.com/StephenQiu30/lanverse/backend/internal/workspace/adapter/postgres"
)

// This explicit protocol fixture proves the consumer's acknowledgment contract,
// not actual downstream impact. Production still refuses an absent owning port.
type bibleImpactProtocol struct{ proof app.ImpactProof }

func (p bibleImpactProtocol) Read(context.Context, identityapp.Principal, uuid.UUID, app.ImpactInput) (app.ImpactProof, error) {
	return p.proof, nil
}
func (p bibleImpactProtocol) Apply(_ context.Context, _ identityapp.Principal, _ uuid.UUID, _ app.ImpactInput, proof app.ImpactProof) error {
	if proof != p.proof {
		return app.ErrConflict
	}
	return nil
}

func TestBiblePGRedirectSplitPinnedVersionsAndCompleteCopyHistory(t *testing.T) {
	db, owner := bibleTestDB(t)
	actor, project := bibleActorProject(t, owner)
	service := app.NewService(bibleStore(db), time.Now)
	createConfirmed := func(name string) app.Receipt {
		t.Helper()
		input := createCharacter(project)
		input.Character.Name = name
		result, err := service.Change(t.Context(), actor, input)
		if err != nil {
			t.Fatal(err)
		}
		confirmed, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "confirm", EntryID: result.EntryID, ExpectedRevision: 1, Key: uuid.New(), RequestID: uuid.New()})
		if err != nil {
			t.Fatal(err)
		}
		return confirmed
	}
	a, b := createConfirmed("王总"), createConfirmed("王先生")
	expected := b.Revision
	merge := app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "merge", EntryID: a.EntryID, ExpectedRevision: a.Revision, TargetID: &b.EntryID, ExpectedTargetRevision: &expected, Key: uuid.New(), RequestID: uuid.New()}
	if _, err := service.Change(t.Context(), actor, merge); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing actual downstream owner reported zero", err)
	}
	proof := app.ImpactProof{SHA256: strings.Repeat("b", 64), Revision: 1}
	protocol := app.NewService(pg.NewStore(db, pg.Factories{Access: func(tx *gorm.DB) app.ProjectAccess { return workspacepg.NewProjectContentAccessStore(tx) }, Impacts: func(_ *gorm.DB) app.Impacts { return bibleImpactProtocol{proof} }}), time.Now)
	if _, err := protocol.Change(t.Context(), actor, merge); err == nil {
		t.Fatal("downstream evidence published without acknowledgment")
	} else {
		var required *app.ImpactConflict
		if !errors.As(err, &required) || required.Proof != proof {
			t.Fatal("lost exact acknowledgment facts", err)
		}
	}
	merge.AcknowledgedImpact = &proof
	merged, err := protocol.Change(t.Context(), actor, merge)
	if err != nil || merged.RedirectID == nil || *merged.RedirectID != b.EntryID {
		t.Fatal("permanent redirect", merged, err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		refs := pg.NewReferences(tx, workspacepg.NewProjectContentAccessStore(tx))
		current, err := refs.Reference(t.Context(), actor, project, a.EntryID, nil)
		if err != nil || current.CharacterID != b.EntryID || current.VersionID != b.VersionID {
			t.Fatal("current alias does not follow confirmed redirect", current, err)
		}
		pinned, err := refs.Reference(t.Context(), actor, project, a.EntryID, &a.VersionID)
		if err != nil || pinned.CharacterID != a.EntryID || pinned.VersionID != a.VersionID {
			t.Fatal("merge rewrote pinned history", pinned, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	child, err := service.Change(t.Context(), actor, app.Command{ProjectID: project, Kind: domain.KindCharacter, Action: "split", EntryID: b.EntryID, ExpectedRevision: b.Revision, Character: &app.CharacterInput{Name: "独立人物", Aliases: []string{"少年王总"}}, Key: uuid.New(), RequestID: uuid.New()})
	if err != nil || child.CreatedEntryID == nil || child.CreatedVersionID == nil || *child.CreatedEntryID == b.EntryID {
		t.Fatal("split did not create independent immutable identity", child, err)
	}
	binding, authority := bibleCopyFixture(t, owner, actor, project)
	snapshot := freezeBibleCopy(t, db, actor, binding, authority)
	if snapshot.Counts.Characters != 3 || snapshot.Counts.CharacterVersions != 3 || snapshot.Counts.CharacterConfirmations != 2 || snapshot.Counts.Redirects != 1 || snapshot.Counts.Splits != 1 {
		t.Fatal("redirect/split historical closure omitted", snapshot.Counts)
	}
	if err := app.NewProjectCopy(bibleCopyStore(db, authority), nil).Transfer(t.Context(), actor, binding, snapshot); err != nil {
		t.Fatal(err)
	}
	register := authority
	register.Phase = "register"
	if err := db.Transaction(func(tx *gorm.DB) error {
		_, err := bibleCopyStore(tx, register).Register(t.Context(), actor, binding, snapshot)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"bible.character_redirect", "bible.character_split"} {
		var n int64
		if err := owner.Table(table).Where("project_id=?", binding.TargetProjectID).Count(&n).Error; err != nil || n != 1 {
			t.Fatal("identity relation missing from target", table, n, err)
		}
	}
}
