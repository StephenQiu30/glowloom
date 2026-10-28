package postgres

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identity "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/operation/application"
)

// ApplyPersonalHistoryCommand changes only the actor's view. Neither execution
// rows nor shared media references are deleted, even for an in-flight record.
func (s *Store) ApplyPersonalHistoryCommand(ctx context.Context, a identity.Principal, key uuid.UUID, in application.PersonalHistoryCommand) (application.PersonalHistoryCommandReceipt, error) {
	receipt := application.PersonalHistoryCommandReceipt{RequestID: key, Action: in.Action, Results: []application.PersonalHistoryCommandResult{}}
	if s == nil || s.db == nil || s.requestAccess == nil {
		return receipt, ErrUnavailable
	}
	if a.ID == uuid.Nil || a.OrgID == uuid.Nil || a.ScopeID == uuid.Nil || a.SessionID == uuid.Nil || a.CredentialEpoch < 1 {
		return receipt, identity.ErrForbidden
	}
	if err := in.Validate(key); err != nil {
		return receipt, err
	}
	body, err := json.Marshal(in)
	if err != nil {
		return receipt, err
	}
	digest := sha256.Sum256(body)
	hash := hex.EncodeToString(digest[:])
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// All personal edits in one space share this lock, so overlapping result and
		// whole-record CAS operations cannot deadlock or lose aggregate revisions.
		lock := "history:" + a.ID.String() + ":" + a.OrgID.String() + ":" + a.ScopeID.String()
		if err := tx.Exec(`SELECT pg_advisory_xact_lock(hashtextextended(?,0))`, lock).Error; err != nil {
			return err
		}
		authorized := map[uuid.UUID]bool{}
		for _, target := range in.Targets {
			exists, err := s.personalHistoryAuthority(tx, a, target.OperationID)
			if err != nil {
				return err
			}
			authorized[target.OperationID] = exists
		}
		var saved struct {
			RequestHash string
			Response    json.RawMessage
		}
		read := tx.Raw(`SELECT request_hash,response FROM operation.personal_history_command_receipt WHERE actor_id=? AND org_id=? AND scope_id=? AND request_id=?`, a.ID, a.OrgID, a.ScopeID, key).Scan(&saved)
		if read.Error != nil {
			return read.Error
		}
		if read.RowsAffected > 0 {
			if saved.RequestHash != hash {
				return application.ErrHistoryCommandConflict
			}
			if err := json.Unmarshal(saved.Response, &receipt); err != nil {
				return err
			}
			for _, result := range receipt.Results {
				if result.OK && !authorized[result.OperationID] {
					return application.ErrPublicNotFound
				}
			}
			return nil
		}
		for _, target := range in.Targets {
			result := application.PersonalHistoryCommandResult{OperationID: target.OperationID, OutputID: target.OutputID, Code: "not_found", AffectedOutputIDs: []uuid.UUID{}}
			if authorized[target.OperationID] {
				var err error
				result, err = s.personalHistoryTarget(tx, a, in, target)
				if err != nil {
					return err
				}
			}
			receipt.Results = append(receipt.Results, result)
		}
		encoded, err := json.Marshal(receipt)
		if err != nil {
			return err
		}
		return tx.Exec(`INSERT INTO operation.personal_history_command_receipt(actor_id,org_id,scope_id,request_id,request_hash,response)VALUES(?,?,?,?,?,?::jsonb)`, a.ID, a.OrgID, a.ScopeID, key, hash, string(encoded)).Error
	})
	return receipt, err
}

func (s *Store) personalHistoryAuthority(tx *gorm.DB, a identity.Principal, id uuid.UUID) (bool, error) {
	var row struct{ ProjectID uuid.UUID }
	query := personalOperationIDs + `SELECT o.project_id FROM operation.operation o
 JOIN personal_operation_ids initiated ON initiated.operation_id=o.id::text
 JOIN workspace.project p ON p.id=o.project_id
 WHERE o.id=? AND p.org_id=? AND p.owner_scope_id=? AND p.status IN ('active','archived')
 AND NOT p.is_delete AND NOT o.is_delete AND o.origin IN ('pipeline','batch','canvas','agent') FOR SHARE OF o,p`
	read := tx.Raw(query, a.ID, a.OrgID, a.ID, a.OrgID, a.ID, a.OrgID, id, a.OrgID, a.ScopeID).Scan(&row)
	if read.Error != nil {
		return false, fmt.Errorf("authorize personal history change: %w", read.Error)
	}
	if read.RowsAffected != 1 {
		return false, nil
	}
	if err := s.requirePublicProject(tx, a, row.ProjectID, false); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) personalHistoryTarget(tx *gorm.DB, a identity.Principal, in application.PersonalHistoryCommand, target application.PersonalHistoryTarget) (application.PersonalHistoryCommandResult, error) {
	result := application.PersonalHistoryCommandResult{OperationID: target.OperationID, OutputID: target.OutputID, Code: "not_found", AffectedOutputIDs: []uuid.UUID{}}
	identityArgs := []any{a.ID, a.OrgID, a.ScopeID, target.OperationID}
	if err := tx.Exec(`INSERT INTO operation.personal_history_record(actor_id,org_id,scope_id,operation_id)VALUES(?,?,?,?)ON CONFLICT DO NOTHING`, identityArgs...).Error; err != nil {
		return result, err
	}
	var record struct {
		Revision int64
		Deleted  bool
	}
	if err := tx.Raw(`SELECT revision,(deleted_at IS NOT NULL) AS deleted FROM operation.personal_history_record WHERE actor_id=? AND org_id=? AND scope_id=? AND operation_id=? FOR UPDATE`, identityArgs...).Scan(&record).Error; err != nil {
		return result, err
	}
	result.Revision = record.Revision
	if record.Deleted {
		result.Code = "deleted"
		return result, nil
	}
	// Lock actual candidates with the operation/project relation intact. A record
	// favorite affects all visible candidates; an empty record is never favoritable.
	var outputs []struct {
		ID       uuid.UUID
		Revision int64
		Favorite bool
		Deleted  bool
	}
	query := `SELECT output.id,COALESCE(meta.revision,0) AS revision,COALESCE(meta.favorite,false) AS favorite,(meta.deleted_at IS NOT NULL) AS deleted
 FROM operation.operation_output output JOIN operation.operation op ON op.id=output.operation_id AND op.project_id=output.project_id
 LEFT JOIN operation.personal_history_output meta ON meta.output_id=output.id AND meta.operation_id=op.id AND meta.actor_id=? AND meta.org_id=? AND meta.scope_id=?
 WHERE output.operation_id=? AND NOT output.is_delete`
	args := append([]any{}, identityArgs...)
	if target.OutputID != nil {
		query += ` AND output.id=?`
		args = append(args, *target.OutputID)
	}
	query += ` ORDER BY output.id FOR SHARE OF output,op`
	if err := tx.Raw(query, args...).Scan(&outputs).Error; err != nil {
		return result, err
	}
	if target.OutputID != nil {
		if len(outputs) != 1 {
			return result, nil
		}
		result.Revision = outputs[0].Revision
		if outputs[0].Deleted {
			result.Code = "deleted"
			return result, nil
		}
	}
	if result.Revision != target.ExpectedRevision {
		result.Code = "revision_conflict"
		return result, nil
	}
	visible := 0
	for _, output := range outputs {
		if !output.Deleted {
			visible++
		}
	}
	if in.Action == "favorite" && visible == 0 {
		result.Code = "no_results"
		return result, nil
	}
	if target.OutputID == nil && in.Action == "delete" {
		if err := tx.Exec(`UPDATE operation.personal_history_record SET deleted_at=now(),revision=revision+1 WHERE actor_id=? AND org_id=? AND scope_id=? AND operation_id=?`, identityArgs...).Error; err != nil {
			return result, err
		}
		result.Revision = record.Revision + 1
		for _, out := range outputs {
			if !out.Deleted {
				result.AffectedOutputIDs = append(result.AffectedOutputIDs, out.ID)
			}
		}
	} else {
		for _, out := range outputs {
			if out.Deleted {
				continue
			}
			favorite := out.Favorite
			deleted := in.Action == "delete"
			if in.Favorited != nil {
				favorite = *in.Favorited
			}
			if deleted {
				favorite = false
			}
			updateArgs := append(append([]any{}, identityArgs...), out.ID, favorite, deleted)
			if err := tx.Exec(`INSERT INTO operation.personal_history_output(actor_id,org_id,scope_id,operation_id,output_id,favorite,deleted_at,revision)
 VALUES(?,?,?,?,?,?,CASE WHEN ? THEN now() ELSE NULL END,1)
 ON CONFLICT(actor_id,org_id,scope_id,operation_id,output_id)DO UPDATE SET favorite=EXCLUDED.favorite,deleted_at=EXCLUDED.deleted_at,revision=personal_history_output.revision+1`, updateArgs...).Error; err != nil {
				return result, err
			}
			result.AffectedOutputIDs = append(result.AffectedOutputIDs, out.ID)
			if target.OutputID != nil {
				result.Revision = out.Revision + 1
			}
		}
		if err := tx.Exec(`UPDATE operation.personal_history_record SET revision=revision+1 WHERE actor_id=? AND org_id=? AND scope_id=? AND operation_id=?`, identityArgs...).Error; err != nil {
			return result, err
		}
		if target.OutputID == nil {
			result.Revision = record.Revision + 1
		}
	}
	result.OK = true
	result.Code = "applied"
	return result, nil
}

var _ application.PersonalHistoryCommandStore = (*Store)(nil)
