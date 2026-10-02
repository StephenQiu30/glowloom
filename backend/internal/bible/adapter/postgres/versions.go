package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/StephenQiu30/lanverse/backend/internal/bible/application"
	"github.com/StephenQiu30/lanverse/backend/internal/bible/domain"
)

func (b *bound) loadHead(ctx context.Context, kind domain.Kind, id uuid.UUID) (domain.Head, error) {
	table, _, _, err := tables(kind)
	if err != nil {
		return domain.Head{}, err
	}
	columns := "id,org_id,project_id,revision,current_version_id,confirmed_version_id,is_delete AS deleted,created_at,updated_at"
	if kind == domain.KindCharacter {
		columns += ",redirect_id"
	}
	var head domain.Head
	read := b.tx.WithContext(ctx).Raw("SELECT "+columns+" FROM "+table+" WHERE org_id=? AND project_id=? AND id=? FOR SHARE", b.actor.OrgID, b.project.ProjectID, id).Scan(&head)
	if read.Error != nil {
		return head, read.Error
	}
	if read.RowsAffected != 1 {
		return head, application.ErrNotFound
	}
	head.Kind = kind
	return head, nil
}

// Load returns the stable head and verified complete immutable current version.
func (b *bound) Load(ctx context.Context, kind domain.Kind, id uuid.UUID) (domain.Head, domain.Version, error) {
	h, err := b.loadHead(ctx, kind, id)
	if err != nil {
		return h, domain.Version{}, err
	}
	v, err := b.LoadVersion(ctx, kind, h.CurrentVersionID)
	if err == nil && v.EntryID != h.ID {
		return h, v, domain.ErrCorruptHistory
	}
	return h, v, err
}

type versionRow struct {
	ID, OrgID, ProjectID, EntryID, ActorID uuid.UUID
	VersionNo                              int64
	PreviousID                             *uuid.UUID
	CreatedAt                              time.Time
	Origin                                 domain.VersionOrigin
	ResultSource                           *string
	Content, ContentSHA256                 string
}

// LoadVersion checks canonical content and all independently stored appearance/reference/voice facts.
func (b *bound) LoadVersion(ctx context.Context, kind domain.Kind, id uuid.UUID) (domain.Version, error) {
	_, table, _, err := tables(kind)
	if err != nil {
		return domain.Version{}, err
	}
	var row versionRow
	read := b.tx.WithContext(ctx).Raw("SELECT id,org_id,project_id,entry_id,version_no,previous_id,actor_id,created_at,origin,result_source::text AS result_source,content::text AS content,content_sha256 FROM "+table+" WHERE org_id=? AND project_id=? AND id=?", b.actor.OrgID, b.project.ProjectID, id).Scan(&row)
	if read.Error != nil {
		return domain.Version{}, read.Error
	}
	if read.RowsAffected != 1 {
		return domain.Version{}, application.ErrNotFound
	}
	v := domain.Version{ID: row.ID, OrgID: row.OrgID, ProjectID: row.ProjectID, EntryID: row.EntryID, Kind: kind, Number: row.VersionNo, PreviousID: row.PreviousID, ActorID: row.ActorID, CreatedAt: row.CreatedAt, Origin: row.Origin, ContentSHA256: row.ContentSHA256}
	if row.ResultSource != nil {
		var source domain.ResultSource
		if err := domain.DecodeClosedJSON([]byte(*row.ResultSource), &source); err != nil {
			return v, domain.ErrCorruptHistory
		}
		v.Result = &source
	}
	switch kind {
	case domain.KindCharacter:
		content, e := domain.DecodeCharacter([]byte(row.Content))
		err = e
		v.Character = &content
	case domain.KindLocation:
		content, e := domain.DecodeLocation([]byte(row.Content))
		err = e
		v.Location = &content
	case domain.KindProp:
		content, e := domain.DecodeProp([]byte(row.Content))
		err = e
		v.Prop = &content
	}
	if err != nil || v.Validate() != nil {
		return v, domain.ErrCorruptHistory
	}
	if kind == domain.KindCharacter {
		if err := b.verifyCharacterRows(ctx, v); err != nil {
			return v, err
		}
	}
	return v, nil
}

// InsertVersion inserts complete canonical content and immutable materialized rows.
func (b *bound) InsertVersion(ctx context.Context, v domain.Version) error {
	if v.OrgID != b.actor.OrgID || v.ProjectID != b.project.ProjectID || v.Validate() != nil {
		return domain.ErrInvalidContent
	}
	_, table, _, err := tables(v.Kind)
	if err != nil {
		return err
	}
	content, _, err := v.Content()
	if err != nil {
		return err
	}
	var result any
	if v.Result != nil {
		encoded, err := json.Marshal(v.Result)
		if err != nil {
			return err
		}
		result = string(encoded)
	}
	if err := exactlyOne(b.tx.WithContext(ctx).Exec("INSERT INTO "+table+"(id,org_id,project_id,entry_id,version_no,previous_id,actor_id,created_at,origin,result_source,content,content_sha256) VALUES(?,?,?,?,?,?,?,?,?,?::jsonb,?::jsonb,?)", v.ID, v.OrgID, v.ProjectID, v.EntryID, v.Number, v.PreviousID, v.ActorID, v.CreatedAt, v.Origin, result, string(content), v.ContentSHA256)); err != nil {
		return err
	}
	if v.Kind != domain.KindCharacter {
		return nil
	}
	for position, look := range v.Character.Looks {
		write := b.tx.Exec(`INSERT INTO bible.look(id,org_id,project_id,character_id,created_at) VALUES(?,?,?,?,?) ON CONFLICT(id) DO NOTHING`, look.ID, v.OrgID, v.ProjectID, v.EntryID, v.CreatedAt)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			var count int64
			if err := b.tx.Raw(`SELECT count(*) FROM bible.look WHERE id=? AND org_id=? AND project_id=? AND character_id=?`, look.ID, v.OrgID, v.ProjectID, v.EntryID).Scan(&count).Error; err != nil {
				return err
			}
			if count != 1 {
				return application.ErrConflict
			}
		}
		lookVersionID := uuid.NewSHA1(v.ID, []byte("look/"+look.ID.String()))
		scopes, err := json.Marshal(look.AppliesTo)
		if err != nil {
			return err
		}
		if err := exactlyOne(b.tx.Exec(`INSERT INTO bible.look_version(id,org_id,project_id,look_id,character_version_id,position,name,description,is_default,applies_to) VALUES(?,?,?,?,?,?,?,?,?,?::jsonb)`, lookVersionID, v.OrgID, v.ProjectID, look.ID, v.ID, position, look.Name, look.Description, look.Default, string(scopes))); err != nil {
			return err
		}
		for pos, ref := range look.References {
			id := uuid.NewSHA1(lookVersionID, []byte("reference/"+string(ref.Role)))
			m := ref.Media
			if err := exactlyOne(b.tx.Exec(`INSERT INTO bible.reference_version(id,org_id,project_id,look_version_id,position,role,media_asset_id,media_revision,media_sha256,media_bytes,rendition_id,rendition_sha256) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, id, v.OrgID, v.ProjectID, lookVersionID, pos, ref.Role, m.AssetID, m.Revision, m.SHA256, m.ByteSize, m.RenditionID, m.RenditionSHA256)); err != nil {
				return err
			}
		}
	}
	if voice := v.Character.Voice; voice != nil {
		encoded, err := json.Marshal(voice)
		if err != nil {
			return err
		}
		if err := exactlyOne(b.tx.Exec(`INSERT INTO bible.voice_version(id,org_id,project_id,character_version_id,source_kind,content) VALUES(?,?,?,?,?,?::jsonb)`, uuid.NewSHA1(v.ID, []byte("voice")), v.OrgID, v.ProjectID, v.ID, voice.Kind, string(encoded))); err != nil {
			return err
		}
	}
	return nil
}

type lookRow struct {
	ID, LookID, CharacterID uuid.UUID
	Position                int
	Name, Description       string
	IsDefault               bool
	AppliesTo               *string
}
type refRow struct {
	ID              uuid.UUID
	Position        int
	Role            domain.ImageRole
	MediaAssetID    uuid.UUID
	MediaRevision   int64
	MediaSHA256     string
	MediaBytes      int64
	RenditionID     uuid.UUID
	RenditionSHA256 string
}

func (b *bound) verifyCharacterRows(ctx context.Context, v domain.Version) error {
	var looks []lookRow
	if err := b.tx.WithContext(ctx).Raw(`SELECT v.id,v.look_id,l.character_id,v.position,v.name,v.description,v.is_default,v.applies_to::text AS applies_to FROM bible.look_version v JOIN bible.look l ON l.id=v.look_id AND l.org_id=v.org_id AND l.project_id=v.project_id WHERE v.org_id=? AND v.project_id=? AND v.character_version_id=? ORDER BY v.position`, v.OrgID, v.ProjectID, v.ID).Scan(&looks).Error; err != nil {
		return err
	}
	if len(looks) != len(v.Character.Looks) {
		return domain.ErrCorruptHistory
	}
	for i, row := range looks {
		look := v.Character.Looks[i]
		expectedID := uuid.NewSHA1(v.ID, []byte("look/"+look.ID.String()))
		if row.ID != expectedID || row.LookID != look.ID || row.CharacterID != v.EntryID || row.Position != i || row.Name != look.Name || row.Description != look.Description || row.IsDefault != look.Default {
			return domain.ErrCorruptHistory
		}
		var scopes []domain.LookScope
		if row.AppliesTo != nil && domain.DecodeClosedJSON([]byte(*row.AppliesTo), &scopes) != nil {
			return domain.ErrCorruptHistory
		}
		a, err := json.Marshal(scopes)
		if err != nil {
			return err
		}
		expected, err := json.Marshal(look.AppliesTo)
		if err != nil {
			return err
		}
		// Canonical LookContent omits both nil and empty applies_to slices. The
		// separately retained row may still contain the client's explicit [].
		// Preserve both immutable representations while comparing their meaning.
		if string(a) != string(expected) && (len(scopes) != 0 || len(look.AppliesTo) != 0) {
			return domain.ErrCorruptHistory
		}
		var refs []refRow
		if err := b.tx.Raw(`SELECT id,position,role,media_asset_id,media_revision,media_sha256,media_bytes,rendition_id,rendition_sha256 FROM bible.reference_version WHERE org_id=? AND project_id=? AND look_version_id=? ORDER BY position`, v.OrgID, v.ProjectID, row.ID).Scan(&refs).Error; err != nil {
			return err
		}
		if len(refs) != len(look.References) {
			return domain.ErrCorruptHistory
		}
		for j, r := range refs {
			expected := look.References[j]
			m := domain.MediaFact{AssetID: r.MediaAssetID, Revision: r.MediaRevision, SHA256: r.MediaSHA256, ByteSize: r.MediaBytes, Kind: "image", RenditionID: r.RenditionID, RenditionSHA256: r.RenditionSHA256}
			if r.Position != j || r.ID != uuid.NewSHA1(row.ID, []byte("reference/"+string(expected.Role))) || r.Role != expected.Role || m != expected.Media {
				return domain.ErrCorruptHistory
			}
		}
	}
	var voiceRows []struct {
		ID         uuid.UUID
		SourceKind domain.VoiceKind
		Content    string
	}
	if err := b.tx.Raw(`SELECT id,source_kind,content::text AS content FROM bible.voice_version WHERE org_id=? AND project_id=? AND character_version_id=?`, v.OrgID, v.ProjectID, v.ID).Scan(&voiceRows).Error; err != nil {
		return err
	}
	if v.Character.Voice == nil {
		if len(voiceRows) != 0 {
			return domain.ErrCorruptHistory
		}
		return nil
	}
	if len(voiceRows) != 1 || voiceRows[0].ID != uuid.NewSHA1(v.ID, []byte("voice")) || voiceRows[0].SourceKind != v.Character.Voice.Kind {
		return domain.ErrCorruptHistory
	}
	var actual domain.VoiceContent
	if domain.DecodeClosedJSON([]byte(voiceRows[0].Content), &actual) != nil || actual.Validate() != nil {
		return domain.ErrCorruptHistory
	}
	encoded, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	expected, err := json.Marshal(v.Character.Voice)
	if err != nil {
		return err
	}
	if string(encoded) != string(expected) {
		return fmt.Errorf("materialized voice disagrees: %w", domain.ErrCorruptHistory)
	}
	return nil
}
