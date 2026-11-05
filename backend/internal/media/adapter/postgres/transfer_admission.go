package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	task "github.com/StephenQiu30/glowloom-server/backend/internal/media/adapter/task"
	"github.com/StephenQiu30/glowloom-server/backend/internal/platform/taskruntime"

	"github.com/google/uuid"
	"gorm.io/gorm"

	identityapp "github.com/StephenQiu30/glowloom-server/backend/internal/identity/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/application"
	"github.com/StephenQiu30/glowloom-server/backend/internal/media/domain"
)

// TransferTopic identifies media-owned permanent commands, never paid operations.
const TransferTopic = "glowloom.media.transfer_command.v1"

func recordTransferCommand(tx *gorm.DB, actor identityapp.Principal, key uuid.UUID, hash string, action, emit application.TransferAction, emitEvent bool, job domain.TransferJob) error {
	response, err := json.Marshal(job)
	if err != nil || len(response) > 1<<20 {
		return application.ErrUnavailable
	}
	var eventID *uuid.UUID
	var eventAction *string
	var eventAttempt *int
	if emitEvent {
		id := uuid.NewSHA1(key, []byte("media-transfer/"+actor.ID.String()+"/"+string(emit)))
		emitted := string(emit)
		eventID, eventAction, eventAttempt = &id, &emitted, &job.Attempt
		delivery := application.TransferDelivery{TransferWorkID: application.TransferWorkID{JobID: job.ID, Attempt: job.Attempt, Reconcile: emit == application.TransferActionReconcile}, EventID: id, RequestID: key, ActorID: actor.ID, OrgID: actor.OrgID, Action: emit}
		body, err := json.Marshal(struct {
			EventID    uuid.UUID                    `json:"event_id"`
			EventType  string                       `json:"event_type"`
			OccurredAt time.Time                    `json:"occurred_at"`
			OrgID      uuid.UUID                    `json:"org_id"`
			Data       application.TransferDelivery `json:"data"`
		}{id, TransferTopic, time.Now().UTC(), actor.OrgID, delivery})
		if err != nil {
			return err
		}
		if emit == application.TransferActionCancel {
			if err := taskruntime.RequestStop(tx.Statement.Context, tx, task.Transfer, map[string]any{"job_id": job.ID, "attempt": job.Attempt}); err != nil {
				return err
			}
		}
		if err := taskruntime.EnqueueReference(tx.Statement.Context, tx, task.Transfer, id.String(), time.Now().UTC(), delivery.TransferWorkID); err != nil {
			return err
		}
		outbox := outboxRecord{ID: id, Topic: TransferTopic, PartitionKey: job.ID.String(), Payload: jsonDocument(body)}
		if err := requireOne(tx.Create(&outbox), application.ErrUnavailable); err != nil {
			return err
		}
	}
	command := transferCommandRecord{ActorID: actor.ID, OrgID: actor.OrgID, IdemKey: key, JobID: job.ID, Action: string(action), RequestSHA256: hash, ResponseBody: response, EventID: eventID, EventAction: eventAction, EventAttempt: eventAttempt}
	if err := requireOne(tx.Create(&command), application.ErrUnavailable); err != nil {
		return err
	}
	safe, _ := json.Marshal(struct {
		ID       uuid.UUID             `json:"id"`
		Revision int64                 `json:"revision"`
		Status   domain.TransferStatus `json:"status"`
		Count    int                   `json:"count"`
	}{job.ID, job.Revision, job.Status, len(job.Items)})
	id := uuid.NewSHA1(key, []byte("media-transfer-audit/"+actor.ID.String()+"/"+string(action)))
	projectID := transferProject(job.Source, job.Target)
	var project *uuid.UUID
	if projectID != uuid.Nil {
		project = &projectID
	}
	audit := uploadAuditRecord{ID: id, OrgID: actor.OrgID, ProjectID: project, ActorID: actor.ID, ActorKind: "user", Action: "media.transfer." + string(action), ObjectType: "media.transfer", ObjectID: job.ID.String(), After: jsonDocument(safe)}
	return requireOne(tx.Omit("RequestID", "CreateTime", "UpdateTime").Create(&audit), application.ErrUnavailable)
}

// CreateTransfer accepts bytes-free SQL facts and permanent target identities.
// Authorization precedes replay, but replay never reruns later catalog CAS.
func (s *TransferStore) CreateTransfer(ctx context.Context, actor identityapp.Principal, in application.TransferInput) (domain.TransferJob, error) {
	var result domain.TransferJob
	if s == nil || s.db == nil {
		return result, application.ErrUnavailable
	}
	if err := in.Validate(); err != nil {
		return result, err
	}
	hash, err := transferHash(in)
	if err != nil {
		return result, err
	}
	var cached *domain.TransferJob
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := transferLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		if _, err := s.access(ctx, tx, actor, in.Source, in.Target, false); err != nil {
			return err
		}
		var err error
		cached, err = s.transferReplay(tx, actor, in.Key, hash, application.TransferActionCreate)
		return err
	})
	if err != nil {
		return result, err
	}
	if cached != nil {
		return *cached, nil
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.requireCurrentActor(tx, actor); err != nil {
			return err
		}
		if err := transferLock(tx, actor.ID, in.Key); err != nil {
			return err
		}
		facts, err := s.access(ctx, tx, actor, in.Source, in.Target, true)
		if err != nil {
			return err
		}
		replay, err := s.transferReplay(tx, actor, in.Key, hash, application.TransferActionCreate)
		if err != nil {
			return err
		}
		if replay != nil {
			result = *replay
			return nil
		}
		if facts.Revision != in.ExpectedProjectRevision {
			return domain.ErrTransferConflict
		}
		if s.guards == nil || s.guards(tx) == nil {
			return application.ErrUnavailable
		}
		busy, err := s.guards(tx).HasInflightProjectWork(ctx, actor, facts.ProjectID)
		if err != nil {
			return err
		}
		if busy {
			return domain.ErrTransferConflict
		}
		source, target, err := s.ensureTransferLibraries(tx, actor, in.Source, in.Target)
		if err != nil {
			return err
		}
		if source.Revision != in.ExpectedSourceRevision || target.Revision != in.ExpectedTargetRevision {
			return domain.ErrTransferConflict
		}
		active, err := storageExists(tx.Model(&transferJobRow{}).Where("(source_library_id IN ? OR target_library_id IN ?) AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", []uuid.UUID{source.ID, target.ID}, []uuid.UUID{source.ID, target.ID}, []domain.TransferStatus{domain.TransferStatusQueued, domain.TransferStatusRunning, domain.TransferStatusNeedsReconciliation, domain.TransferStatusCancelRequested}, true, false))
		if err != nil {
			return err
		}
		if active {
			return domain.ErrTransferConflict
		}
		folders, err := libraryFolders(tx, target.ID, true)
		if err != nil {
			return err
		}
		if in.TargetFolderID != nil {
			found := false
			for _, f := range folders {
				if f.ID == *in.TargetFolderID && f.Revision == in.ExpectedFolderRevision {
					found = true
				}
			}
			if !found {
				return domain.ErrTransferConflict
			}
		}
		ids := make([]uuid.UUID, 0, len(in.Items))
		for _, item := range in.Items {
			ids = append(ids, item.ID)
		}
		sourceActor, targetActor, err := s.transferActors(tx, actor, in.Source, in.Target, true)
		if err != nil {
			return err
		}
		items, err := lockLibraryItems(tx, sourceActor, in.Source, source.ID, ids)
		if err != nil {
			return err
		}
		jobID := uuid.NewSHA1(in.Key, []byte("media-transfer-job/"+actor.ID.String()))
		now := s.clock().UTC().Truncate(time.Microsecond)
		if now.IsZero() {
			return application.ErrUnavailable
		}
		frozen := make([]application.FrozenTransferItem, 0, len(in.Items))
		workIntents := make([]*uuid.UUID, 0, len(in.Items))
		for _, wanted := range in.Items {
			item := items[wanted.ID]
			if item.Revision != wanted.Revision || item.State != domain.LibraryItemStateActive {
				return domain.ErrTransferConflict
			}
			var file *application.LibraryMediaFile
			if item.AssetID != nil {
				sourceFile, err := transferSourceFile(tx, sourceActor, in.Source, *item.AssetID)
				if err != nil {
					return err
				}
				file = &sourceFile
			}
			prepared, err := application.PrepareTransferItem(jobID, targetActor.OrgID, actor.ID, in.Target, in.TargetFolderID, item, file, now)
			if err != nil {
				return err
			}
			prepared, err = transferAttemptItem(prepared, 1)
			if err != nil {
				return err
			}
			if len(prepared.Objects) > 0 && !s.routed {
				return application.ErrStorageNotReady
			}
			work, err := bindTransferAttempt(ctx, tx, jobID, target.ID, len(frozen), 1, nil, &prepared)
			if err != nil {
				return err
			}
			workIntents = append(workIntents, work)
			prepared.TargetItem.Position = int64(len(frozen))
			frozen = append(frozen, prepared)
		}
		manifest, bodies, err := transferFrozenBytes(frozen)
		if err != nil {
			return err
		}
		jobRecord := transferJobRow{ID: jobID, OrgID: actor.OrgID, ActorID: actor.ID, SourceScopeID: &sourceActor.ScopeID, TargetScopeID: &targetActor.ScopeID, SourceOrgID: &sourceActor.OrgID, TargetOrgID: &targetActor.OrgID, SourceLibraryID: source.ID, TargetLibraryID: target.ID, SourceKind: string(in.Source.Kind), TargetKind: string(in.Target.Kind), SourceProjectID: in.Source.ProjectID, TargetProjectID: in.Target.ProjectID, TargetFolderID: in.TargetFolderID, TargetFolderRevision: in.ExpectedFolderRevision, SourceRevision: source.Revision, TargetRevision: target.Revision, ProjectRevision: facts.Revision, ManifestSHA256: manifest, ItemCount: len(frozen), Status: domain.TransferStatusQueued, Stage: domain.TransferStageFrozen, Attempt: 1, Revision: 1, ProcessEnded: true, CreatedAt: now, UpdatedAt: now}
		if err := tx.Create(&jobRecord).Error; err != nil {
			return err
		}
		for index, item := range frozen {
			var sourceID, targetID *uuid.UUID
			if item.SourceAsset != nil {
				sourceID = &item.SourceAsset.ID
				targetID = &item.TargetAsset.ID
			}
			itemRecord := transferItemRow{JobID: jobID, ItemIndex: index, SourceItemID: item.SourceItem.ID, TargetItemID: item.TargetItem.ID, SourceAssetID: sourceID, TargetAssetID: targetID, Frozen: bodies[index], Status: domain.TransferItemStatusQueued}
			if err := tx.Create(&itemRecord).Error; err != nil {
				return err
			}
			if err := insertTransferAttempt(tx, jobID, index, 1, workIntents[index], item); err != nil {
				return err
			}
		}
		row, err := s.readJob(tx, actor, jobID, false, true)
		if err != nil {
			return err
		}
		result, err = transferView(tx, row)
		if err != nil {
			return err
		}
		return recordTransferCommand(tx, actor, in.Key, hash, application.TransferActionCreate, application.TransferActionStart, true, result)
	})
	if err != nil {
		return domain.TransferJob{}, fmt.Errorf("admit media transfer: %w", err)
	}
	return result, nil
}

// HasInflightWork is consumed by workspace lifecycle and other admission guards.
// It reads actual transfer state for every creator in the already locked project.
func (s *TransferStore) HasInflightWork(ctx context.Context, actor identityapp.Principal, project uuid.UUID) (bool, error) {
	if s == nil || s.db == nil {
		return false, application.ErrUnavailable
	}
	if project == uuid.Nil || actor.ID == uuid.Nil || actor.OrgID == uuid.Nil {
		return false, identityapp.ErrForbidden
	}
	busy, err := storageExists(s.db.WithContext(ctx).Model(&transferJobRow{}).Where("org_id = ? AND (source_project_id = ? OR target_project_id = ?) AND (status IN ? OR execution_unconfirmed = ? OR process_ended = ?)", actor.OrgID, project, project, []domain.TransferStatus{domain.TransferStatusQueued, domain.TransferStatusRunning, domain.TransferStatusNeedsReconciliation, domain.TransferStatusCancelRequested}, true, false))
	return busy, err
}
