package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/lanverse/backend/internal/identity/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/application"
	"github.com/StephenQiu30/lanverse/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/lanverse/backend/internal/workspace/application"
)

// ProjectAccessFactory binds workspace authorization to the exact script transaction.
type ProjectAccessFactory func(*gorm.DB) application.ProjectAccess

// SourceStore holds immutable command and content facts with current owner authorization.
type SourceStore struct {
	db              *gorm.DB
	access          ProjectAccessFactory
	importAuthority *application.ImportAuthority
	importRecovery  bool
}

// NewSourceStore injects the own database and the workspace transaction factory.
func NewSourceStore(db *gorm.DB, access ProjectAccessFactory) *SourceStore {
	return &SourceStore{db: db, access: access}
}

func (s *SourceStore) transaction(ctx context.Context, actor identityapp.Principal, project uuid.UUID, write bool, f func(*gorm.DB, application.ProjectAccess, workspaceapp.ProjectContentAccess) error) error {
	if s == nil || s.db == nil || s.access == nil {
		return application.ErrUnavailable
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		access := s.access(tx)
		if access == nil {
			return application.ErrUnavailable
		}
		facts, err := access.Authorize(ctx, actor, project, write)
		if errors.Is(err, workspaceapp.ErrProjectNotFound) {
			return errors.Join(application.ErrNotFound, err)
		}
		if err != nil {
			return err
		}
		if facts.ProjectID != project || facts.OrgID != actor.OrgID {
			return application.ErrUnavailable
		}
		if write && s.importAuthority != nil {
			if err := authorizeImportPublication(tx, actor, project, *s.importAuthority); err != nil {
				return err
			}
		}
		return f(tx, access, facts)
	})
}

// LoadBase reads explicit old versions or the current draft without creating a head.
func (s *SourceStore) LoadBase(ctx context.Context, actor identityapp.Principal, project uuid.UUID, version *uuid.UUID) (application.SourceBase, error) {
	var result application.SourceBase
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		state, err := readState(tx, actor.OrgID, project, false)
		if err != nil {
			return err
		}
		result.State = state
		id := version
		if id == nil {
			id = state.DraftVersionID
		}
		if id == nil {
			result.Sources = []domain.SourceRecord{}
			return nil
		}
		v, err := readVersion(tx, actor.OrgID, project, *id)
		if err != nil {
			return err
		}
		result.Version = &v
		result.Sources, err = readSources(tx, actor.OrgID, project, v.SourceIDs)
		return err
	})
	return result, err
}

func commandLock(tx *gorm.DB, actor, key uuid.UUID) error {
	var locked int
	return tx.Raw(`SELECT 1 FROM pg_advisory_xact_lock(69420,hashtext(?))`, actor.String()+":"+key.String()).Scan(&locked).Error
}
func findCommand(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, hash string) (*application.PendingWrite, error) {
	if err := checkRequestScope(tx, actor, project, key, "", hash); err != nil {
		return nil, err
	}
	var row struct {
		OrgID, ProjectID  uuid.UUID
		RequestHash, Plan string
		Response          *string
	}
	read := tx.Raw(`SELECT c.org_id,c.project_id,c.request_hash,c.plan::text AS plan,r.response::text AS response FROM script.command c LEFT JOIN script.command_result r ON r.actor_id=c.actor_id AND r.request_id=c.request_id WHERE c.actor_id=? AND c.request_id=?`, actor.ID, key).Scan(&row)
	if read.Error != nil {
		return nil, fmt.Errorf("read permanent script command: %w", read.Error)
	}
	if read.RowsAffected == 0 {
		return nil, nil
	}
	if row.OrgID != actor.OrgID || row.ProjectID != project || row.RequestHash != hash {
		return nil, application.ErrIdempotencyConflict
	}
	var saved application.PendingWrite
	if err := json.Unmarshal([]byte(row.Plan), &saved.Plan); err != nil {
		return nil, application.ErrUnavailable
	}
	if row.Response != nil {
		var response application.SourceReceipt
		if err := json.Unmarshal([]byte(*row.Response), &response); err != nil {
			return nil, application.ErrUnavailable
		}
		saved.Receipt = &response
	}
	return &saved, nil
}

// FindCommand reauthorizes before replay; a new trace ID cannot alter its first result.
func (s *SourceStore) FindCommand(ctx context.Context, actor identityapp.Principal, project, key uuid.UUID, hash string) (*application.PendingWrite, error) {
	var result *application.PendingWrite
	err := s.transaction(ctx, actor, project, false, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		var err error
		result, err = findCommand(tx, actor, project, key, hash)
		return err
	})
	return result, err
}

func validatePlan(actor identityapp.Principal, p application.WritePlan) error {
	if p.ActorID != actor.ID || p.OrgID != actor.OrgID || p.Command.ProjectID == uuid.Nil || p.Command.Key == uuid.Nil || p.Command.ExpectedRevision < 0 || p.CreatedAt.IsZero() || len(p.RequestHash) != 64 || len(p.Command.Sources) != 0 {
		return domain.ErrInvalidSource
	}
	if p.Version.OrgID != actor.OrgID || p.Version.ProjectID != p.Command.ProjectID || p.Version.ID == uuid.Nil || p.Candidate.VersionID != p.Version.ID || p.Candidate.OrgID != actor.OrgID || p.Candidate.ProjectID != p.Version.ProjectID {
		return domain.ErrInvalidSource
	}
	expected := make([]domain.ObjectFact, 0, len(p.NewSources)*2+2)
	for _, source := range p.NewSources {
		if source.OrgID != actor.OrgID || source.ProjectID != p.Command.ProjectID || source.RightsActorID != actor.ID {
			return domain.ErrInvalidSource
		}
		expected = append(expected, source.Original, source.Rich)
	}
	if !p.Reuse {
		expected = append(expected, p.Version.Text, p.Version.Rich)
	} else if len(p.NewSources) != 0 {
		return domain.ErrInvalidSource
	}
	if !slices.Equal(expected, p.Objects) {
		return domain.ErrInvalidSource
	}
	for _, object := range p.Objects {
		if err := object.Validate(); err != nil || !strings.HasPrefix(object.Key, "projects/"+p.Command.ProjectID.String()+"/script/") {
			return domain.ErrInvalidSource
		}
	}
	return nil
}

func checkHead(state domain.ProjectState, p application.WritePlan) error {
	if state.Revision != p.Command.ExpectedRevision {
		return application.ErrConflict
	}
	if state.DraftVersionID == nil && p.Command.BaseVersionID != nil || state.DraftVersionID != nil && (p.Command.BaseVersionID == nil || *state.DraftVersionID != *p.Command.BaseVersionID) {
		return application.ErrConflict
	}
	return nil
}

// BeginWrite freezes the original server-chosen IDs and object digest intents atomically.
func (s *SourceStore) BeginWrite(ctx context.Context, actor identityapp.Principal, p application.WritePlan) (application.PendingWrite, error) {
	var result application.PendingWrite
	if err := validatePlan(actor, p); err != nil {
		return result, err
	}
	for _, source := range p.NewSources {
		if source.Origin == "file" && s.importAuthority == nil {
			return result, identityapp.ErrForbidden
		}
	}
	err := s.transaction(ctx, actor, p.Command.ProjectID, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := commandLock(tx, actor.ID, p.Command.Key); err != nil {
			return err
		}
		prior, err := findCommand(tx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash)
		if err != nil {
			return err
		}
		if prior != nil {
			result = *prior
			return nil
		}
		state, err := readState(tx, actor.OrgID, p.Command.ProjectID, true)
		if err != nil {
			return err
		}
		if err := checkHead(state, p); err != nil {
			return err
		}
		if s.importAuthority == nil {
			if err := rejectPendingWrite(tx, actor.OrgID, p.Command.ProjectID); err != nil {
				return err
			}
		} else {
			if err := validateImportedPlan(tx, p, *s.importAuthority); err != nil {
				return err
			}
			if err := rejectPendingSourceExcept(tx, actor.OrgID, p.Command.ProjectID, uuid.Nil); err != nil {
				return err
			}
			if err := rejectPendingImport(tx, actor.OrgID, p.Command.ProjectID, s.importAuthority.JobID); err != nil {
				return err
			}
		}
		if err := reuseOrNumberVersion(tx, actor, state, &p); err != nil {
			return err
		}
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if err := registerRequest(tx, actor, p.Command.ProjectID, p.Command.Key, "source_"+p.Command.Action, p.RequestHash, p.CreatedAt); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.command(actor_id,request_id,org_id,project_id,action,request_hash,plan,created_at) VALUES(?,?,?,?,?,?,?::jsonb,?)`, actor.ID, p.Command.Key, actor.OrgID, p.Command.ProjectID, p.Command.Action, p.RequestHash, string(data), p.CreatedAt)); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.command_state(actor_id,request_id,id,status,updated_at) VALUES(?,?,?,'pending',?)`, actor.ID, p.Command.Key, sourceIntentID(p), p.CreatedAt)); err != nil {
			return err
		}
		for _, object := range p.Objects {
			if err := exactlyOne(tx.Exec(`INSERT INTO script.object_intent(actor_id,request_id,org_id,project_id,object_key,sha256,byte_size,mime) VALUES(?,?,?,?,?,?,?,?)`, actor.ID, p.Command.Key, actor.OrgID, p.Command.ProjectID, object.Key, object.SHA256, object.ByteSize, object.MIME)); err != nil {
				return err
			}
			if err := exactlyOne(tx.Exec(`INSERT INTO script.object_state(object_key,updated_at) VALUES(?,?)`, object.Key, p.CreatedAt)); err != nil {
				return err
			}
		}
		result.Plan = p
		return nil
	})
	return result, err
}

// FinishWrite commits all immutable sources/version, both heads, events and first receipt.
func (s *SourceStore) FinishWrite(ctx context.Context, actor identityapp.Principal, p application.WritePlan) (application.SourceReceipt, error) {
	var result application.SourceReceipt
	err := s.transaction(ctx, actor, p.Command.ProjectID, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := commandLock(tx, actor.ID, p.Command.Key); err != nil {
			return err
		}
		prior, err := findCommand(tx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash)
		if err != nil {
			return err
		}
		if prior == nil {
			return application.ErrNotFound
		}
		if prior.Receipt != nil {
			result = *prior.Receipt
			return nil
		}
		if !p.Reuse {
			if err := requireSourceIO(tx, p, nil, true); err != nil {
				return err
			}
		} else {
			if err := requirePendingSource(tx, p); err != nil {
				return err
			}
		}
		if err := validatePlan(actor, p); err != nil {
			return err
		}
		frozen, err := json.Marshal(prior.Plan)
		if err != nil {
			return err
		}
		given, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if !slices.Equal(frozen, given) {
			return application.ErrIdempotencyConflict
		}
		state, err := readState(tx, actor.OrgID, p.Command.ProjectID, true)
		if err != nil {
			return err
		}
		if err := checkHead(state, p); err != nil {
			return err
		}
		if p.Reuse {
			result, err = finishReusedWrite(ctx, tx, access, actor, project, state, p)
			return err
		}
		// Current source version numbers derive from the immutable chronological history.
		var latest int64
		if err := tx.Raw(`SELECT coalesce(max(version_no),0) FROM script.script_version WHERE org_id=? AND project_id=?`, actor.OrgID, p.Command.ProjectID).Scan(&latest).Error; err != nil {
			return err
		}
		if p.Version.VersionNo != latest+1 {
			return application.ErrConflict
		}
		for _, source := range p.NewSources {
			if err := insertSource(ctx, tx, source); err != nil {
				return err
			}
		}
		if err := insertVersion(tx, p.Version, p.Candidate); err != nil {
			return err
		}
		if state.Revision == 0 {
			if err := exactlyOne(tx.Exec(`INSERT INTO script.project_state(project_id,org_id,revision,draft_version_id,updated_at) VALUES(?,?,1,?,?)`, p.Command.ProjectID, actor.OrgID, p.Version.ID, p.CreatedAt)); err != nil {
				return err
			}
		} else {
			if err := exactlyOne(tx.Exec(`UPDATE script.project_state SET revision=revision+1,draft_version_id=?,updated_at=? WHERE project_id=? AND org_id=? AND revision=?`, p.Version.ID, p.CreatedAt, p.Command.ProjectID, actor.OrgID, state.Revision)); err != nil {
				return err
			}
		}
		projectRevision, err := access.TouchContent(ctx, actor, p.Command.ProjectID, project.Revision)
		if err != nil {
			return err
		}
		result = application.SourceReceipt{ScriptRevision: state.Revision + 1, ProjectRevision: projectRevision, VersionID: p.Version.ID, SplitSetID: p.Candidate.ID, Mappings: p.Mappings, Changed: true}
		response, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`INSERT INTO script.command_result(actor_id,request_id,response,created_at) VALUES(?,?,?::jsonb,?)`, actor.ID, p.Command.Key, string(response), p.CreatedAt)); err != nil {
			return err
		}
		if err := exactlyOne(tx.Exec(`UPDATE script.command_state SET status='completed',needs_reconciliation=false,revision=revision+1,updated_at=? WHERE actor_id=? AND request_id=? AND status='pending' AND NOT cancellation_requested`, p.CreatedAt, actor.ID, p.Command.Key)); err != nil {
			return err
		}
		if s.importAuthority != nil {
			if err := finishImportedPublication(tx, actor, p, result, *s.importAuthority); err != nil {
				return err
			}
		}
		return sourceAudit(tx, actor, p, result)
	})
	return result, err
}
