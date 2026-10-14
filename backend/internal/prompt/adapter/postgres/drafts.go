package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"time"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type DraftStore struct {
	db     *gorm.DB
	access func(*gorm.DB) application.DraftAccess
}

func NewDraftStore(db *gorm.DB, access func(*gorm.DB) application.DraftAccess) *DraftStore {
	return &DraftStore{db, access}
}
func (s *DraftStore) authorize(ctx context.Context, tx *gorm.DB, a identityapp.Principal, p uuid.UUID, write bool) error {
	if a.ID == uuid.Nil || p == uuid.Nil {
		return application.ErrDraftNotFound
	}
	if s.access == nil {
		return errUnavailable
	}
	return s.access(tx).AuthorizeDraft(ctx, a, p, write)
}
func (s *DraftStore) ReadDraft(ctx context.Context, a identityapp.Principal, p, id uuid.UUID) (application.DraftSnapshot, error) {
	var out application.DraftSnapshot
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, false); err != nil {
			return err
		}
		var err error
		out, err = readDraft(tx, a.ID, p, id)
		return err
	})
	return out, err
}
func readDraft(tx *gorm.DB, actor, p, id uuid.UUID) (application.DraftSnapshot, error) {
	var raw struct {
		DraftID              uuid.UUID
		Revision             int64
		DraftHash, InputHash string
		Draft                []byte
		UpdatedAt            time.Time
	}
	r := tx.Raw(`SELECT id AS draft_id,revision,draft_hash,input_hash,draft,updated_at FROM workspace.prompt_draft WHERE id=? AND project_id=? AND actor_id=? FOR SHARE`, id, p, actor).Scan(&raw)
	if r.Error != nil {
		return application.DraftSnapshot{}, fmt.Errorf("read prompt draft: %w", r.Error)
	}
	if r.RowsAffected != 1 {
		return application.DraftSnapshot{}, application.ErrDraftNotFound
	}
	draft, err := domain.DecodeDraft(raw.Draft)
	return application.DraftSnapshot{DraftID: raw.DraftID, Revision: raw.Revision, DraftHash: raw.DraftHash, InputHash: raw.InputHash, Draft: draft, UpdatedAt: raw.UpdatedAt}, err
}
func (s *DraftStore) SaveDraft(ctx context.Context, a identityapp.Principal, in application.DraftWrite) (application.DraftSnapshot, error) {
	if in.Key == uuid.Nil || in.ExpectedRevision < 0 || in.ExpectedRevision >= math.MaxInt32 || (in.ExpectedRevision > 0 && in.DraftID == uuid.Nil) {
		return application.DraftSnapshot{}, domain.ErrInvalidDraft
	}
	if err := in.Draft.Validate(); err != nil {
		return application.DraftSnapshot{}, err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return application.DraftSnapshot{}, err
	}
	hash := sha256.Sum256(body)
	fingerprint := hex.EncodeToString(hash[:])
	var out application.DraftSnapshot
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, in.ProjectID, true); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69360,hashtext(?))`, a.ID.String()+":"+in.Key.String()).Error; err != nil {
			return err
		}
		var receipt struct {
			RequestHash string
			Response    []byte
		}
		r := tx.Raw(`SELECT request_hash,response FROM workspace.prompt_draft_receipt WHERE actor_id=? AND request_key=?`, a.ID, in.Key).Scan(&receipt)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if receipt.RequestHash != fingerprint {
				return application.ErrIdempotencyConflict
			}
			return json.Unmarshal(receipt.Response, &out)
		}
		if in.DraftID == uuid.Nil {
			in.DraftID = uuid.New()
		} else {
			var revision int64
			r = tx.Raw(`SELECT revision FROM workspace.prompt_draft WHERE id=? AND project_id=? AND actor_id=? FOR UPDATE`, in.DraftID, in.ProjectID, a.ID).Scan(&revision)
			if r.Error != nil {
				return r.Error
			}
			if r.RowsAffected != 1 {
				return application.ErrDraftNotFound
			}
			if revision != in.ExpectedRevision {
				return application.ErrRevisionConflict
			}
			previous, err := readDraft(tx, a.ID, in.ProjectID, in.DraftID)
			if err != nil {
				return err
			}
			if err := in.Draft.ExtendsLabels(previous.Draft.LabelState); err != nil {
				return err
			}
		}
		if err := ValidateDraftSources(tx, a, in.ProjectID, in.Draft, false); err != nil {
			return err
		}
		raw, err := json.Marshal(in.Draft)
		if err != nil {
			return err
		}
		dh, ih := in.Draft.Hashes()
		out = application.DraftSnapshot{DraftID: in.DraftID, Revision: in.ExpectedRevision + 1, DraftHash: dh, InputHash: ih, Draft: in.Draft}
		r = tx.Raw(`INSERT INTO workspace.prompt_draft(id,project_id,actor_id,revision,draft,draft_hash,input_hash) VALUES(?,?,?,?,?::jsonb,?,?) ON CONFLICT(id) DO UPDATE SET revision=excluded.revision,draft=excluded.draft,draft_hash=excluded.draft_hash,input_hash=excluded.input_hash,updated_at=now() WHERE workspace.prompt_draft.revision=? AND workspace.prompt_draft.actor_id=? RETURNING updated_at`, in.DraftID, in.ProjectID, a.ID, out.Revision, string(raw), dh, ih, in.ExpectedRevision, a.ID).Scan(&out.UpdatedAt)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return application.ErrRevisionConflict
		}
		saved, err := json.Marshal(out)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO workspace.prompt_draft_receipt(actor_id,request_key,project_id,request_hash,response) VALUES(?,?,?,?,?::jsonb)`, a.ID, in.Key, in.ProjectID, fingerprint, string(saved)).Error
	})
	return out, err
}
func (s *DraftStore) ListDrafts(ctx context.Context, a identityapp.Principal, p uuid.UUID, cursor string) (application.DraftPage, error) {
	out := application.DraftPage{Items: []application.DraftSnapshot{}}
	after := uuid.Nil
	if cursor != "" {
		var err error
		after, err = uuid.Parse(cursor)
		if err != nil {
			return out, domain.ErrInvalidDraft
		}
	}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.authorize(ctx, tx, a, p, false); err != nil {
			return err
		}
		var ids []uuid.UUID
		if err := tx.Raw(`SELECT id FROM workspace.prompt_draft WHERE project_id=? AND actor_id=? AND id>? ORDER BY id LIMIT 21`, p, a.ID, after).Scan(&ids).Error; err != nil {
			return err
		}
		if len(ids) > 20 {
			out.NextCursor = ids[19].String()
			ids = ids[:20]
		}
		for _, id := range ids {
			draft, err := readDraft(tx, a.ID, p, id)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, draft)
		}
		return nil
	})
	return out, err
}
