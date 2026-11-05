package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/script/domain"
	workspaceapp "github.com/StephenQiu30/glowloom-server/backend/internal/workspace/application"
)

// ProjectAccessFactory binds workspace authorization to the exact script transaction.
type ProjectAccessFactory func(*gorm.DB) application.ProjectAccess

// SourceStore holds immutable command and content facts with current owner authorization.
type SourceStore struct {
	db              *gorm.DB
	access          ProjectAccessFactory
	importAuthority *application.ImportAuthority
	importRecovery  bool
	characters      CharacterReferenceFactory
	locations       SourceObjectLocationFactory
	publication     PublicationAuthorizationFactory
}

// NewSourceStore injects the own database and the workspace transaction factory.
func NewSourceStore(db *gorm.DB, access ProjectAccessFactory) *SourceStore {
	return &SourceStore{db: db, access: access}
}

// CharacterReferenceFactory binds confirmed Bible facts to the exact script transaction.
type CharacterReferenceFactory func(*gorm.DB) application.CharacterReferences

// NewSourceStoreWithCharacters injects the own confirmed-character consumer without changing old nil contracts.
func NewSourceStoreWithCharacters(db *gorm.DB, access ProjectAccessFactory, characters CharacterReferenceFactory) *SourceStore {
	return &SourceStore{db: db, access: access, characters: characters}
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
		if errors.Is(err, workspaceapp.ErrProjectNotFound) || errors.Is(err, workspaceapp.ErrScopeNotFound) {
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

// registerCommand runs after write authorization holds the workspace project lock.
// The unique immutable request records scope/hash; it must never require UPDATE
// permission just to serialize a write which is already locked by its owner.
func registerCommand(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, action, hash string, now time.Time) error {
	return registerRequest(tx, actor, project, key, action, hash, now)
}

func findCommand(tx *gorm.DB, actor identityapp.Principal, project, key uuid.UUID, hash string) (*application.PendingWrite, error) {
	if err := checkRequestScope(tx, actor, project, key, "", hash); err != nil {
		return nil, err
	}
	var row scriptCommandRow
	if err := tx.Where("actor_id = ? AND request_id = ?", actor.ID, key).Take(&row).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	} else if err != nil {
		return nil, fmt.Errorf("read permanent script command: %w", err)
	}
	if row.OrgID != actor.OrgID || row.ProjectID != project || row.RequestHash != hash {
		return nil, application.ErrIdempotencyConflict
	}
	var saved application.PendingWrite
	if err := json.Unmarshal(row.Plan, &saved.Plan); err != nil {
		return nil, application.ErrUnavailable
	}
	if err := loadPlanLocations(tx, &saved.Plan); err != nil {
		return nil, err
	}
	var responseRow scriptCommandResultRow
	if err := tx.Where("actor_id = ? AND request_id = ?", actor.ID, key).Take(&responseRow).Error; err == nil {
		var response application.SourceReceipt
		if err := json.Unmarshal(responseRow.Response, &response); err != nil {
			return nil, application.ErrUnavailable
		}
		saved.Receipt = &response
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
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
		if source.Origin == domain.SourceOriginFile && s.importAuthority == nil {
			return result, identityapp.ErrForbidden
		}
	}
	err := s.commandTransaction(ctx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash, true, func(tx *gorm.DB, _ application.ProjectAccess, _ workspaceapp.ProjectContentAccess) error {
		if err := registerCommand(tx, actor, p.Command.ProjectID, p.Command.Key, "source_"+string(p.Command.Action), p.RequestHash, p.CreatedAt); err != nil {
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
		if err := s.admitSourceLocations(ctx, tx, &p); err != nil {
			return err
		}
		data, err := json.Marshal(p)
		if err != nil {
			return err
		}
		if len(data) > domain.MaxWritePlanBytes {
			return domain.ErrInvalidSource
		}
		if err := registerRequest(tx, actor, p.Command.ProjectID, p.Command.Key, "source_"+string(p.Command.Action), p.RequestHash, p.CreatedAt); err != nil {
			return err
		}
		command := scriptCommandRow{ActorID: actor.ID, RequestID: p.Command.Key, OrgID: actor.OrgID, ProjectID: p.Command.ProjectID, Action: string(p.Command.Action), RequestHash: p.RequestHash, Plan: scriptJSON(data), CreatedAt: p.CreatedAt}
		if err := exactlyOne(tx.Create(&command)); err != nil {
			return err
		}
		commandState := scriptCommandStateRow{ActorID: actor.ID, RequestID: p.Command.Key, ID: sourceIntentID(p), Status: scriptCommandPending, Revision: 1, IOState: scriptIOIdle, UpdatedAt: p.CreatedAt}
		if err := exactlyOne(tx.Create(&commandState)); err != nil {
			return err
		}
		for _, object := range p.Objects {
			if object.LocationID == uuid.Nil {
				return application.ErrObjectMismatch
			}
			intent := scriptObjectIntentRow{ActorID: actor.ID, RequestID: p.Command.Key, OrgID: actor.OrgID, ProjectID: p.Command.ProjectID, ObjectKey: object.Key, SHA256: object.SHA256, ByteSize: object.ByteSize, MIME: object.MIME, ObjectLocationID: object.LocationID}
			if err := exactlyOne(tx.Create(&intent)); err != nil {
				return err
			}
			state := scriptObjectStateRow{ObjectKey: object.Key, UpdatedAt: p.CreatedAt, ObjectLocationID: object.LocationID}
			if err := exactlyOne(tx.Create(&state)); err != nil {
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
	err := s.commandTransaction(ctx, actor, p.Command.ProjectID, p.Command.Key, p.RequestHash, true, func(tx *gorm.DB, access application.ProjectAccess, project workspaceapp.ProjectContentAccess) error {
		if err := registerCommand(tx, actor, p.Command.ProjectID, p.Command.Key, "source_"+string(p.Command.Action), p.RequestHash, p.CreatedAt); err != nil {
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
		if !slices.Equal(prior.Plan.Objects, p.Objects) {
			return application.ErrIdempotencyConflict
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
		var latestVersion versionRow
		if err := tx.Model(&versionRow{}).Select("version_no").Where("org_id = ? AND project_id = ?", actor.OrgID, p.Command.ProjectID).Order("version_no DESC").Take(&latestVersion).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			latestVersion.VersionNo = 0
		} else if err != nil {
			return err
		}
		if p.Version.VersionNo != latestVersion.VersionNo+1 {
			return application.ErrConflict
		}
		if len(p.Objects) > 0 {
			if p.StorageIntentID == uuid.Nil || s.locations == nil || s.locations(tx) == nil {
				return application.ErrUnavailable
			}
			if err := s.locations(tx).Accept(ctx, p.StorageIntentID, p.Objects); err != nil {
				return err
			}
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
			projectState := scriptProjectStateRow{ProjectID: p.Command.ProjectID, OrgID: actor.OrgID, Revision: 1, DraftVersionID: &p.Version.ID, UpdatedAt: p.CreatedAt}
			if err := exactlyOne(tx.Create(&projectState)); err != nil {
				return err
			}
		} else {
			updated := tx.Model(&scriptProjectStateRow{}).Where("project_id = ? AND org_id = ? AND revision = ?", p.Command.ProjectID, actor.OrgID, state.Revision).
				Updates(map[string]any{"revision": state.Revision + 1, "draft_version_id": p.Version.ID, "updated_at": p.CreatedAt})
			if err := exactlyOne(updated); err != nil {
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
		commandResult := scriptCommandResultRow{ActorID: actor.ID, RequestID: p.Command.Key, Response: scriptJSON(response), CreatedAt: p.CreatedAt}
		if err := exactlyOne(tx.Create(&commandResult)); err != nil {
			return err
		}
		var commandState scriptCommandStateRow
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("actor_id = ? AND request_id = ?", actor.ID, p.Command.Key).Take(&commandState).Error; err != nil {
			return err
		}
		if commandState.Status != scriptCommandPending || commandState.CancellationRequested {
			return application.ErrConflict
		}
		updated := tx.Model(&scriptCommandStateRow{}).
			Where("actor_id = ? AND request_id = ? AND status = ? AND cancellation_requested = ?", actor.ID, p.Command.Key, scriptCommandPending, false).
			Updates(map[string]any{"status": scriptCommandCompleted, "needs_reconciliation": false, "revision": commandState.Revision + 1, "updated_at": p.CreatedAt})
		if err := exactlyOne(updated); err != nil {
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
