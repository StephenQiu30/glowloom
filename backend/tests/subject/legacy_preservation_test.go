package subject_test

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

	biblewire "github.com/StephenQiu30/glowloom-server/backend/internal/app/bible"
	subjectwire "github.com/StephenQiu30/glowloom-server/backend/internal/app/subject"
	bible "github.com/StephenQiu30/glowloom-server/backend/internal/bible/domain"
	prompt "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	subject "github.com/StephenQiu30/glowloom-server/backend/internal/subject/application"
)

// This is legacy read compatibility, not a migration: a location with no image
// remains in Bible, using its original identity, canonical content and hash.
func TestLegacyBibleLocationPreservesLongNameHashAndDraftReference(t *testing.T) {
	runtime, owner := subjectPostgres(t)
	actor := subjectSession(t, owner)
	project := subjectProject(t, owner, actor)
	entry, version := uuid.New(), uuid.New()
	legacy := bible.LocationContent{Name: strings.Repeat("旧", 511) + "😀", Description: "保留没有主参考图的项目历史场景", Prompt: "旧场景原始提示"}
	canonical, originalSHA, err := bible.EncodeLocation(legacy)
	if err != nil || utf8.RuneCountInString(legacy.Name) != 512 {
		t.Fatal("canonical legacy location fixture", err)
	}
	created := time.Now().UTC().Truncate(time.Microsecond)
	if err := owner.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO bible.location(id,org_id,project_id,revision,current_version_id,created_at,updated_at) VALUES(?,?,?,1,?,?,?)`, entry, actor.OrgID, project, version, created, created).Error; err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO bible.location_version(id,org_id,project_id,entry_id,version_no,actor_id,created_at,origin,content,content_sha256) VALUES(?,?,?,?,1,?,?,'manual',?::jsonb,?)`, version, actor.OrgID, project, entry, actor.ID, created, string(canonical), originalSHA).Error
	}); err != nil {
		t.Fatal("seed exact old identity and version", err)
	}
	type legacyRows struct {
		Head, Version []byte
		SHA256        string
	}
	readRows := func() legacyRows {
		t.Helper()
		var rows legacyRows
		read := runtime.Raw(`SELECT to_jsonb(h)::text AS head,to_jsonb(v)::text AS version,v.content_sha256 AS sha256 FROM bible.location h JOIN bible.location_version v ON v.id=h.current_version_id AND v.entry_id=h.id WHERE h.id=? AND v.id=?`, entry, version).Scan(&rows)
		if read.Error != nil || read.RowsAffected != 1 || rows.SHA256 != originalSHA {
			t.Fatal("read exact retained legacy rows", read.Error)
		}
		return rows
	}
	before := readRows()
	newStore := subjectwire.NewStore(runtime, subjectwire.Objects{})
	assertLegacy := func(stage string) {
		t.Helper()
		if _, known, err := newStore.ReadVersionIfKnown(t.Context(), actor, entry, version); err != nil || known {
			t.Fatal(stage, "legacy identity was taken over by new Subject", known, err)
		}
		old, err := biblewire.ProvideBibleStore(runtime, nil, nil).Version(t.Context(), actor, project, bible.KindLocation, entry, version)
		if err != nil || old.ID != version || old.EntryID != entry || old.Location == nil || old.Location.Name != legacy.Name || old.ContentSHA256 != originalSHA {
			t.Fatal(stage, "old typed reader truncated or rehashed history", err)
		}
		raw, sha, err := old.Content()
		if err != nil || !bytes.Equal(raw, canonical) || sha != originalSHA {
			t.Fatal(stage, "old canonical content changed", err)
		}
		after := readRows()
		if !bytes.Equal(after.Head, before.Head) || !bytes.Equal(after.Version, before.Version) {
			t.Fatal(stage, "legacy head/version storage changed")
		}
	}
	assertLegacy("before new subject")
	draft := promptdomain.Draft{SchemaVersion: 1, Params: map[string]any{},
		Document:   promptdomain.Document{Type: "doc", Content: []promptdomain.Paragraph{{Type: "paragraph", Content: []promptdomain.Inline{{Type: "text", Text: "旧场景"}, {Type: "reference", Attrs: &promptdomain.ReferenceAttrs{ReferenceID: "legacy-location"}}}}}},
		References: []promptdomain.Reference{{ID: "legacy-location", BindingGeneration: 1, MediaKind: "image", Role: "subject", Order: 0, Source: promptdomain.ReferenceSource{Kind: "subject", SourceID: entry.String(), SourceVersion: version.String(), Description: legacy.Name + "：" + legacy.Description, Media: []promptdomain.SourceMedia{}}}},
		LabelState: promptdomain.LabelState{HighWater: map[string]int64{"image": 0, "video": 0, "audio": 0, "subject": 1}, Allocations: map[string]promptdomain.LabelAllocation{"legacy-location": {LabelKind: "subject", Ordinal: 1}}},
	}
	drafts := actualSubjectDraftStore(runtime)
	saved, err := drafts.SaveDraft(t.Context(), actor, prompt.DraftWrite{ProjectID: project, Key: uuid.New(), Draft: draft})
	if err != nil {
		t.Fatal("real new-wired draft store rejected no-image legacy source", err)
	}
	storage := subjectStorage(t)
	asset, _, _ := subjectStoredPNG(t, owner, actor, storage)
	createdSubject, err := subject.NewService(subjectwire.NewStore(runtime, storage.objects), time.Now).Save(t.Context(), actor, uuid.Nil, uuid.New(), subject.SaveInput{Name: "合法新主体", Primary: subject.MediaSelection{AssetID: asset}})
	if err != nil || createdSubject.ID == entry || createdSubject.VersionID == version {
		t.Fatal("create distinct legal new Subject", err)
	}
	if _, known, err := newStore.ReadVersionIfKnown(t.Context(), actor, createdSubject.ID, createdSubject.VersionID); err != nil || !known {
		t.Fatal("legal new identity did not remain distinct from legacy fallback", known, err)
	}
	assertLegacy("after new subject")
	updated, err := drafts.SaveDraft(t.Context(), actor, prompt.DraftWrite{ProjectID: project, DraftID: saved.DraftID, ExpectedRevision: saved.Revision, Key: uuid.New(), Draft: draft})
	if err != nil || updated.InputHash != saved.InputHash {
		t.Fatal("legacy resolver stopped accepting original fixed reference", err)
	}
	readback, err := drafts.ReadDraft(t.Context(), actor, project, saved.DraftID)
	if err != nil || len(readback.Draft.References) != 1 {
		t.Fatal("read original legacy draft", err)
	}
	source := readback.Draft.References[0].Source
	if source.SourceID != entry.String() || source.SourceVersion != version.String() || source.Description != draft.References[0].Source.Description || len(source.Media) != 0 {
		t.Fatal("legacy reference identity/name or absent primary was silently converted")
	}
	compiled, err := readback.Draft.Compile()
	if err != nil || len(compiled.Media) != 0 || !strings.Contains(compiled.Prompt, legacy.Name) {
		t.Fatal("legacy compiler truncated name or invented physical media", err)
	}
	assertLegacy("after repeated draft save")
}
