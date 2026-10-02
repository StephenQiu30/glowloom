package bible_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	app "github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func copyBibleHistory(t *testing.T) (app.ProjectCopyBinding, app.CopyHistory, []app.ReferenceMapping) {
	t.Helper()
	b := app.ProjectCopyBinding{JobID: uuid.New(), OrgID: uuid.New(), SourceProjectID: uuid.New(), TargetProjectID: uuid.New()}
	now := time.Now().UTC().Truncate(time.Microsecond)
	character, version := uuid.New(), uuid.New()
	content := characterContent()
	m := domain.MediaFact{AssetID: uuid.New(), Revision: 7, SHA256: strings.Repeat("a", 64), ByteSize: 40, Kind: "image", RenditionID: uuid.New(), RenditionSHA256: strings.Repeat("b", 64)}
	content.Looks[0].References = []domain.ImageReference{{Role: domain.RoleFront, Media: m}}
	_, sha, err := domain.EncodeCharacter(content)
	if err != nil {
		t.Fatal(err)
	}
	v := domain.Version{ID: version, EntryID: character, OrgID: b.OrgID, ProjectID: b.SourceProjectID, Kind: domain.KindCharacter, Number: 1, ActorID: uuid.New(), CreatedAt: now, Origin: domain.OriginManual, ContentSHA256: sha, Character: &content}
	h := domain.Head{ID: character, OrgID: b.OrgID, ProjectID: b.SourceProjectID, Kind: domain.KindCharacter, Revision: 2, CurrentVersionID: version, ConfirmedVersionID: &version, CreatedAt: now, UpdatedAt: now}
	history := app.CopyHistory{Heads: []domain.Head{h}, Versions: []domain.Version{v}, Looks: []app.CopyLook{{ID: content.Looks[0].ID, CharacterID: character, CreatedAt: now}}, Confirmations: []app.CopyConfirmation{{Kind: domain.KindCharacter, Confirmation: domain.Confirmation{ID: uuid.New(), EntryID: character, VersionID: version, Revision: 2, ActorID: v.ActorID, CreatedAt: now}}}, Redirects: []domain.Redirect{}, Splits: []domain.Split{}}
	target := m
	target.AssetID = uuid.New()
	target.RenditionID = uuid.New()
	target.Revision = 1
	return b, history, []app.ReferenceMapping{{Source: m, Target: target}}
}

func TestBibleCopyCompleteHistoricalMappingsAndSourceHashIntegrity(t *testing.T) {
	b, h, refs := copyBibleHistory(t)
	manifest, err := app.RemapProjectHistory(b, h, refs, nil)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Counts.Characters != 1 || manifest.Counts.CharacterVersions != 1 || manifest.Counts.CharacterConfirmations != 1 || manifest.Counts.LookVersions != 1 || manifest.Counts.References != 1 || len(manifest.Identities) != 1 || len(manifest.Versions) != 1 {
		t.Fatal("partial manifest", manifest.Counts)
	}
	old, target := h.Versions[0], manifest.Target.Versions[0]
	if old.ID == target.ID || old.EntryID == target.EntryID || old.Character.Looks[0].ID == target.Character.Looks[0].ID || target.Character.Looks[0].References[0].Media != refs[0].Target || target.ContentSHA256 == old.ContentSHA256 || target.Character.Name != old.Character.Name {
		t.Fatal("own/foreign identities mixed")
	}
	if err := target.Validate(); err != nil {
		t.Fatal("target hash not regenerated", err)
	}
	h.Versions[0].ContentSHA256 = strings.Repeat("c", 64)
	if _, err := app.RemapProjectHistory(b, h, refs, nil); !errors.Is(err, domain.ErrCorruptHistory) {
		t.Fatal("corrupt source hash hidden by remap", err)
	}
}

func TestBibleCopyRejectsMissingForeignClosureAndDanglingConfirmedVersion(t *testing.T) {
	b, h, refs := copyBibleHistory(t)
	if _, err := app.RemapProjectHistory(b, h, nil, nil); !errors.Is(err, app.ErrUnavailable) {
		t.Fatal("missing media mapping silently removed", err)
	}
	unknown := uuid.New()
	h.Heads[0].ConfirmedVersionID = &unknown
	if _, err := app.RemapProjectHistory(b, h, refs, nil); !errors.Is(err, domain.ErrCorruptHistory) {
		t.Fatal("unknown confirmed version", err)
	}
}
