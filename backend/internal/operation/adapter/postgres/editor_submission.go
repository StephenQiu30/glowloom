package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
	promptdomain "github.com/StephenQiu30/glowloom-server/backend/internal/prompt/domain"
)

func (s *Store) lockEditorOwner(ctx context.Context, tx *gorm.DB, a identityapp.Principal, p uuid.UUID, source promptdomain.InputSource) (application.SubmissionCurrent, error) {
	var out application.SubmissionCurrent
	if s.editorSnapshots == nil || !source.Valid() {
		return out, application.ErrInvalidFreeQuote
	}
	if err := tx.Exec(`INSERT INTO operation.prompt_submission_slot(actor_id,project_id,owner_key) VALUES(?,?,?) ON CONFLICT DO NOTHING`, a.ID, p, source.OwnerKey()).Error; err != nil {
		return out, err
	}
	var slot struct {
		Epoch        int64
		SubmissionID *uuid.UUID
	}
	if err := tx.Raw(`SELECT epoch,submission_id FROM operation.prompt_submission_slot WHERE actor_id=? AND project_id=? AND owner_key=? FOR UPDATE`, a.ID, p, source.OwnerKey()).Scan(&slot).Error; err != nil {
		return out, err
	}
	if err := s.editorSnapshots(tx).AuthorizeOwner(ctx, a, p, source); err != nil {
		return out, err
	}
	out.Epoch = slot.Epoch
	if slot.SubmissionID != nil {
		sub, err := readEditorSubmission(tx, a, p, *slot.SubmissionID)
		if err != nil {
			return out, err
		}
		out.Submission = &sub
	}
	return out, nil
}
func readEditorSubmission(tx *gorm.DB, a identityapp.Principal, p, id uuid.UUID) (application.Submission, error) {
	var row struct {
		ID, OperationID, ConfirmKey  uuid.UUID
		Epoch                        int64
		InputSource, Snapshot, Quote []byte
		State                        string
	}
	r := tx.Raw(`SELECT id,operation_id,confirm_key,epoch,input_source,snapshot,quote,state FROM operation.prompt_submission WHERE id=? AND actor_id=? AND project_id=?`, id, a.ID, p).Scan(&row)
	if r.Error != nil {
		return application.Submission{}, r.Error
	}
	if r.RowsAffected != 1 {
		return application.Submission{}, ErrNotFound
	}
	out := application.Submission{ID: row.ID, Epoch: row.Epoch, OperationID: row.OperationID, ConfirmKey: row.ConfirmKey, State: row.State}
	if err := json.Unmarshal(row.InputSource, &out.InputSource); err != nil {
		return out, err
	}
	if err := json.Unmarshal(row.Snapshot, &out.Draft); err != nil {
		return out, err
	}
	var quote application.CreateFreeQuoteResult
	if err := json.Unmarshal(row.Quote, &quote); err != nil {
		return out, err
	}
	out.Quote = &application.EditorQuote{CompilerVersion: quote.CompilerVersion, ReferenceMapping: quote.ReferenceMapping, QuoteMicros: quote.QuoteMicros, AvailableMicros: quote.AvailableMicros, FinalPrompt: quote.FinalPrompt, ExpiresAt: quote.ExpiresAt, Confirmable: quote.Confirmable}
	return out, nil
}

// CurrentSubmission discovers the current private slot without relying on browser state.
func (s *Store) CurrentSubmission(ctx context.Context, a identityapp.Principal, p uuid.UUID, source promptdomain.InputSource) (application.SubmissionCurrent, error) {
	var out application.SubmissionCurrent
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireProject(tx, a, p, true); err != nil {
			return err
		}
		var err error
		out, err = s.lockEditorOwner(ctx, tx, a, p, source)
		return err
	})
	return out, err
}
func recordEditorSubmission(tx *gorm.DB, a identityapp.Principal, p uuid.UUID, source promptdomain.InputSource, epoch int64, draft promptdomain.Draft, result *application.CreateFreeQuoteResult) error {
	if epoch >= math.MaxInt64-1 {
		return application.ErrSubmissionEpoch
	}
	id, key := uuid.New(), uuid.New()
	result.SubmissionID = &id
	result.ConfirmKey = &key
	result.SubmissionEpoch = epoch + 1
	result.EditorDraft = &draft
	compiled, err := draft.Compile()
	if err != nil {
		return err
	}
	result.CompilerVersion = promptdomain.CompilerVersion
	result.ReferenceMapping = compiled.Mapping
	rawSource, err := json.Marshal(source)
	if err != nil {
		return err
	}
	rawDraft, err := json.Marshal(draft)
	if err != nil {
		return err
	}
	rawQuote, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := tx.Exec(`INSERT INTO operation.prompt_submission(id,actor_id,project_id,owner_key,epoch,operation_id,confirm_key,input_source,snapshot,quote,state) VALUES(?,?,?,?,?,?,?,?::jsonb,?::jsonb,?::jsonb,'quoted')`, id, a.ID, p, source.OwnerKey(), epoch+1, result.OperationID, key, string(rawSource), string(rawDraft), string(rawQuote)).Error; err != nil {
		return err
	}
	return tx.Exec(`UPDATE operation.prompt_submission_slot SET epoch=?,submission_id=? WHERE actor_id=? AND project_id=? AND owner_key=?`, epoch+1, id, a.ID, p, source.OwnerKey()).Error
}
func compileEditorInput(tx *gorm.DB, input application.CreateFreeQuoteInput, d promptdomain.Draft) (application.CreateFreeQuoteInput, error) {
	if d.ModelVersion == nil || d.Mode == nil {
		return input, application.ErrFreeQuoteInputNotReady
	}
	version, err := uuid.Parse(*d.ModelVersion)
	if err != nil {
		return input, application.ErrFreeQuoteInputNotReady
	}
	var model struct{ ModelKey, Capability string }
	r := tx.Raw(`SELECT m.model_key,m.capability FROM catalog.model_profile m
 JOIN catalog.provider p ON p.id=m.provider_id
 LEFT JOIN catalog.model_release r ON r.id=m.active_release_id AND r.status='active' AND r.model_id=m.id
 WHERE CASE WHEN p.adapter_key IN ('openrouter','volcengine_ark') THEN r.model_version_id ELSE m.current_version_id END=?
 AND m.status='active' AND NOT m.is_delete AND NOT p.is_delete FOR SHARE OF m,p`, version).Scan(&model)
	if r.Error != nil {
		return input, r.Error
	}
	if r.RowsAffected != 1 {
		return input, application.ErrFreeQuoteModelMissing
	}
	compiled, err := d.Compile()
	if err != nil {
		return input, err
	}
	params := map[string]any{}
	for k, v := range d.Params {
		if k != "output_count" {
			params[k] = v
		}
	}
	count := float64(1)
	if v, ok := d.Params["output_count"]; ok {
		var valid bool
		count, valid = v.(float64)
		if !valid || math.Trunc(count) != count || count < 1 || count > 8 {
			return input, application.ErrFreeQuoteInputNotReady
		}
	}
	raw, err := json.Marshal(params)
	if err != nil {
		return input, err
	}
	prepared := application.FreeQuoteItemInput{ModelKey: model.ModelKey, Capability: model.Capability, Mode: *d.Mode, Prompt: compiled.Prompt, Params: raw, OutputCount: int32(count), ForceRegenerate: input.ForceRegenerate}
	for _, m := range compiled.Media {
		id, err := uuid.Parse(m.SourceID)
		if err != nil {
			return input, promptdomain.ErrReferenceUnavailable
		}
		prepared.MediaInputs = append(prepared.MediaInputs, application.FreeQuoteMediaInput{Role: m.Role, MediaAssetID: id})
	}
	input.FreeQuoteItemInput = prepared
	return input, input.Validate()
}

// lockEditorConfirmation serializes confirmation with discard and validates the current saved intent.
func (s *Store) lockEditorConfirmation(ctx context.Context, tx *gorm.DB, a identityapp.Principal, input application.ConfirmSingleQuoteInput) (*uuid.UUID, error) {
	var identity struct{ ID uuid.UUID }
	r := tx.Raw(`SELECT id FROM operation.prompt_submission WHERE operation_id=?`, input.OperationID).Scan(&identity)
	if r.Error != nil {
		return nil, r.Error
	}
	if r.RowsAffected == 0 {
		return nil, nil
	}
	id := identity.ID
	sub, err := readEditorSubmission(tx, a, input.ProjectID, id)
	if err != nil {
		return nil, err
	}
	current, err := s.lockEditorOwner(ctx, tx, a, input.ProjectID, sub.InputSource)
	if err != nil {
		return nil, err
	}
	if input.RequestID != sub.ConfirmKey.String() || current.Submission == nil || current.Submission.ID != id || current.Submission.State != "quoted" {
		return nil, application.ErrSubmissionEpoch
	}
	if _, err := s.editorSnapshots(tx).ReadInput(ctx, a, input.ProjectID, sub.InputSource, true); err != nil {
		return nil, err
	}
	return &id, nil
}

// ResolveSubmission serializes explicit closing with quote confirmation.
func (s *Store) ResolveSubmission(ctx context.Context, a identityapp.Principal, in application.SubmissionResolution) (application.SubmissionCurrent, error) {
	var out application.SubmissionCurrent
	if in.Key == uuid.Nil || in.ID == uuid.Nil || in.ExpectedEpoch < 1 || (in.Action != "acknowledge_result" && in.Action != "discard_quote") {
		return out, application.ErrInvalidFreeQuote
	}
	raw, _ := json.Marshal(in)
	hash := sha256.Sum256(raw)
	fingerprint := hex.EncodeToString(hash[:])
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireProject(tx, a, in.ProjectID, true); err != nil {
			return err
		}
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(69360,hashtext(?))`, a.ID.String()+":"+in.Key.String()).Error; err != nil {
			return err
		}
		sub, err := readEditorSubmission(tx, a, in.ProjectID, in.ID)
		if err != nil {
			return err
		}
		current, err := s.lockEditorOwner(ctx, tx, a, in.ProjectID, sub.InputSource)
		if err != nil {
			return err
		}
		var receipt struct {
			RequestHash string
			Response    []byte
		}
		r := tx.Raw(`SELECT request_hash,response FROM operation.prompt_resolution_receipt WHERE actor_id=? AND request_key=?`, a.ID, in.Key).Scan(&receipt)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected > 0 {
			if receipt.RequestHash != fingerprint {
				return application.ErrQuoteKeyReused
			}
			return json.Unmarshal(receipt.Response, &out)
		}
		if current.Epoch != in.ExpectedEpoch || current.Submission == nil || current.Submission.ID != in.ID {
			return application.ErrSubmissionEpoch
		}
		var op struct {
			Status        string
			ReservationID *uuid.UUID
		}
		r = tx.Raw(`SELECT status,reservation_id FROM operation.operation WHERE id=? FOR UPDATE`, sub.OperationID).Scan(&op)
		if r.Error != nil {
			return r.Error
		}
		if r.RowsAffected != 1 {
			return ErrNotFound
		}
		if in.Action == "discard_quote" {
			if op.ReservationID != nil || current.Submission.State == "accepted" {
				return application.ErrSubmissionAccepted
			}
			if op.Status != "quoted" && op.Status != "expired" {
				return application.ErrSubmissionEpoch
			}
			if err := tx.Exec(`UPDATE operation.operation SET status='expired',failure_code='quote_discarded',update_time=now() WHERE id=?`, sub.OperationID).Error; err != nil {
				return err
			}
		} else if op.ReservationID == nil && op.Status != "expired" {
			return application.ErrSubmissionUnresolved
		}
		if current.Epoch >= math.MaxInt64-1 {
			return application.ErrSubmissionEpoch
		}
		if err := tx.Exec(`UPDATE operation.prompt_submission SET state='closed',resolution=? WHERE id=?`, in.Action, in.ID).Error; err != nil {
			return err
		}
		out.Epoch = current.Epoch + 1
		if err := tx.Exec(`UPDATE operation.prompt_submission_slot SET epoch=?,submission_id=NULL WHERE actor_id=? AND project_id=? AND owner_key=?`, out.Epoch, a.ID, in.ProjectID, sub.InputSource.OwnerKey()).Error; err != nil {
			return err
		}
		body, err := json.Marshal(out)
		if err != nil {
			return fmt.Errorf("encode submission resolution: %w", err)
		}
		return tx.Exec(`INSERT INTO operation.prompt_resolution_receipt(actor_id,request_key,request_hash,response) VALUES(?,?,?,?::jsonb)`, a.ID, in.Key, fingerprint, string(body)).Error
	})
	return out, err
}
